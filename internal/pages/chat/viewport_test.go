package chat

import (
	"image"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/ggdream/microlight/assets/locales"
	timeline "github.com/ggdream/microlight/internal/activity"
	"github.com/ggdream/microlight/internal/attachment"
	"github.com/ggdream/microlight/internal/localtools"
	"github.com/ggdream/microlight/internal/pages/settings"
)

func TestCompactImagePreviewFitsAboveKeyboard(t *testing.T) {
	const alt = "Portrait fixture"
	bitmap := ui.NewBitmap(image.NewRGBA(image.Rect(0, 0, 200, 600)))
	app := &chatApp{imagePreview: chatImagePreview{image: &markdownImage{bitmap: bitmap}, alt: alt}}
	window := ui.NewTester(app.renderImagePreview, 320, 660)
	window.SetInsets(50, 8, 310, 8)
	closeButton, ok := window.Find(locales.ClosePreview())
	if !ok || closeButton.H < 44 || closeButton.Y < 50 {
		t.Fatalf("preview close action is too small or outside the safe area: %+v", closeButton)
	}
	picture, ok := window.Find(alt)
	if !ok || picture.Y < closeButton.Y+closeButton.H || picture.Y+picture.H > 350 ||
		picture.W*3 < picture.H-1 || picture.W*3 > picture.H+1 {
		t.Fatalf("the image must fit below the actions and retain its proportions: %+v", picture)
	}
	if err := window.Click(locales.ClosePreview()); err != nil || app.imagePreview.image != nil {
		t.Fatalf("could not close the preview: %v", err)
	}
}

func TestCompactAttachmentRemoveDoesNotOpenPreview(t *testing.T) {
	app := &chatApp{}
	file := &attachment.File{Name: "notes.txt", MIME: "text/plain", Data: []byte("Notes")}
	removed := false
	window := ui.NewTester(func(c *ui.Context) {
		app.attachmentTile(c, file, 44, func() { removed = true })
	}, 320, 200)
	button, ok := window.Find(locales.RemoveAttachment(file.Name))
	preview, visible := window.Find(locales.AttachmentLabel(file.Name))
	if !ok || !visible || button.W < 44 || button.H < 44 || button.X+button.W != preview.X+preview.W || button.Y != preview.Y {
		t.Fatalf("remove action is too small: %+v", button)
	}
	x, y := button.X+button.W/2, button.Y+button.H/2
	window.FingerDown(x, y)
	window.FingerUp(x, y)
	if !removed || app.previewDirectory != "" {
		t.Fatal("tapping remove must remove the attachment without opening a preview")
	}
}

func TestCompactApprovalKeepsAllActionsTouchable(t *testing.T) {
	app := &chatApp{}
	approval := &localtools.Approval{Target: strings.Repeat("/directory", 20), Preview: "Example command"}
	window := ui.NewTester(func(c *ui.Context) {
		app.approvalControls(c, &conversation{}, &turn{}, approval, true)
	}, 320, 330)
	window.SetInsets(0, 8, 100, 8)
	for _, name := range []string{locales.RejectOperation(), locales.ApproveOnce(), locales.Stop()} {
		rect, ok := window.Find(name)
		if !ok || rect.H < 44 || rect.X < 8 || rect.X+rect.W > 312 || rect.Y+rect.H > 230 {
			t.Errorf("action %q falls outside the usable area: %+v", name, rect)
		}
	}
}

func TestNarrowFilePreviewKeepsContentAndChangesAccessible(t *testing.T) {
	content := "File preview content"
	entry := &turn{Activity: []timeline.Entry{{Tool: localtools.Write, Target: "fixture.txt", Output: &content}}}
	app := &chatApp{}
	app.openToolPreview(entry, 0)
	window := ui.NewTester(func(c *ui.Context) {
		width, _ := c.Size()
		app.toolPreviewPane(c, width, "")
	}, 320, 180)
	body, ok := window.Find(content)
	if !ok || body.Y+body.H > 180 {
		t.Fatalf("file content is outside the short preview: %+v", body)
	}
	closeButton, ok := window.Find(locales.ClosePreview())
	if !ok || closeButton.H < 44 || closeButton.X+closeButton.W > 320 {
		t.Fatalf("close action must fit the narrow header: %+v", closeButton)
	}
	if err := window.Click(locales.RoundChanges()); err != nil || app.toolPanel.active != nil {
		t.Fatalf("opening the changes tab failed: %v", err)
	}
	if err := window.Click("fixture.txt"); err != nil || app.toolPanel.active == nil {
		t.Fatalf("opening a file from the changes tree failed: %v", err)
	}
}

func TestNarrowWindowNavigationPreservesDraft(t *testing.T) {
	app := &chatApp{backend: &Chat{}}
	app.draft.SetText("Keep this draft")
	window := ui.NewTester(app.view, 1120, 760)
	if !window.HasText(locales.ResizeSidebar()) {
		t.Fatal("wide window should show the sidebar")
	}
	window.SetSize(390, 720)
	if window.HasText(locales.ResizeSidebar()) {
		t.Fatal("narrow window should hide the fixed sidebar")
	}
	tap := func(label string) {
		t.Helper()
		rect, ok := window.Find(label)
		if !ok || rect.W <= 0 || rect.H <= 0 {
			t.Fatalf("cannot tap %q", label)
		}
		x, y := rect.X+rect.W/2, rect.Y+rect.H/2
		window.FingerDown(x, y)
		window.FingerUp(x, y)
	}
	tap(locales.ChatHistory())
	if !app.compactHistory || !window.HasText(locales.NoChats()) {
		t.Fatal("history should open on touch")
	}
	tap(locales.Settings())
	if !app.settingsOpen || !window.HasText(locales.SettingsGeneral()) {
		t.Fatal("settings should open from compact history")
	}
	tap(locales.Settings())
	tap(locales.AppAbout())
	if !window.HasText(locales.RepositoryAddress()) {
		t.Fatal("the compact settings menu should open the selected section")
	}
	tap(locales.BackToChat())
	if app.settingsOpen || !app.compactHistory {
		t.Fatal("leaving settings should return to compact history")
	}
	tap(locales.BackToChat())
	if app.compactHistory {
		t.Fatal("back should return to chat")
	}
	tap(locales.ChatHistory())
	scrim, ok := window.Find(locales.CollapseSidebar())
	if !ok || scrim.X > 304 || scrim.W < 56 {
		t.Fatalf("the drawer should leave a dismissible area beside it: %+v", scrim)
	}
	tap(locales.CollapseSidebar())
	if app.compactHistory {
		t.Fatal("tapping outside the drawer should return to chat")
	}
	window.SetSize(810, 900)
	if !window.HasText(locales.ResizeSidebar()) || app.draft.Markdown() != "Keep this draft" {
		t.Fatal("tablet width should restore the sidebar and preserve the draft")
	}
}

func TestShortWindowKeepsDialogActionsVisible(t *testing.T) {
	chat := &conversation{ID: newID(), Title: "Sample chat", workspace: strings.Repeat("/long-segment", 30)}
	app := &chatApp{chatAction: &conversationAction{chat: chat, info: true}}
	window := ui.NewTester(app.conversationActionDialog, 320, 210)
	closeLabel := locales.CloseChatInfo()
	button, ok := window.Find(closeLabel)
	if !ok || button.Y < 0 || button.Y+button.H > 210 {
		t.Fatalf("dialog close action is outside the short window: %+v", button)
	}
	if err := window.Click(closeLabel); err != nil || app.chatAction != nil {
		t.Fatalf("could not close the dialog: %v", err)
	}
	app.fullAccess = &fullAccessRequest{}
	access := ui.NewTester(app.fullAccessDialog, 320, 210)
	if err := access.Click(locales.Cancel()); err != nil || app.fullAccess != nil {
		t.Fatalf("could not cancel the permission dialog: %v", err)
	}
}

func TestShortModelSettingsCanScrollPastProviderCard(t *testing.T) {
	app := &chatApp{}
	app.loadSettings()
	app.openSettings()
	app.settingsTab = settings.Model
	window := ui.NewTester(app.settingsPage, 390, 280)
	provider, ok := window.Find(locales.ModelProviders())
	if !ok || provider.Y < 0 || provider.Y+provider.H > 280 {
		t.Fatalf("provider card is outside the window: %+v", provider)
	}
	save, ok := window.Find(locales.SaveSettings())
	if !ok || save.Y < 0 || save.Y+save.H > 280 {
		t.Fatalf("save action is outside the window: %+v", save)
	}
	window.Scroll(195, 135, 0, 225)
	window.Advance(time.Second)
	input, ok := window.Find(locales.BaseURL())
	if !ok || input.Y < 52 || input.Y+input.H > save.Y {
		t.Fatalf("could not scroll to the base URL field: %+v", input)
	}
}

func TestShortWindowKeepsChatTypographyAndTouchTarget(t *testing.T) {
	app := &chatApp{backend: &Chat{}}
	var textSize float32
	window := ui.NewTester(func(c *ui.Context) {
		app.view(c)
		textSize = c.Theme().FontSize
	}, 390, 760)
	tall, ok := window.Find(locales.ChatHistory())
	if !ok {
		t.Fatal("missing history control")
	}
	originalTextSize := textSize
	window.SetSize(390, 480)
	short, ok := window.Find(locales.ChatHistory())
	if !ok || short.Y >= tall.Y || short.H < 44 || textSize != originalTextSize {
		t.Fatalf("height-only resize must compact the header and preserve text and hit area: tall=%+v short=%+v font=%v→%v", tall, short, originalTextSize, textSize)
	}
	window.SetSize(320, 480)
	window.Frame()
	if textSize >= originalTextSize {
		t.Fatal("width-only resize should update inherited text sizes")
	}
}

func TestCompactComposerFitsSingleLineWithModelControls(t *testing.T) {
	app := &chatApp{
		backend:  &Chat{},
		settings: modelSettings{Model: "a-very-long-provider-model-identifier"},
		providers: providerCatalog{
			Selected:  modelChoice{ProviderID: "provider"},
			Providers: []modelProvider{{ID: "provider", Name: "Provider"}},
		},
	}
	window := ui.NewTester(func(c *ui.Context) { app.composer(c, nil) }, 320, 540)
	editor, ok := window.Find(locales.Message())
	if !ok || editor.H < 30 || editor.H > 42 {
		t.Fatalf("the empty composer should reserve one line: %+v", editor)
	}
	var right float32
	for _, label := range []string{locales.AddAttachment(), locales.AgentPermissions(), locales.ChooseChatModel()} {
		control, ok := window.Find(label)
		if !ok || control.X < right || control.X+control.W > 320 || control.H > 44 {
			t.Fatalf("control %q should fit the compact toolbar: %+v", label, control)
		}
		right = control.X + control.W
	}
	send, ok := window.Find(locales.Send())
	if !ok || send.Y >= editor.Y+editor.H || send.X < editor.X+editor.W {
		t.Fatalf("single-line send must sit beside the editor: %+v", send)
	}
}

func TestCompactSelectorsUseDismissibleSheets(t *testing.T) {
	store, err := openLocalStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	app := &chatApp{
		store:     store,
		settings:  modelSettings{Protocol: openAIChat, BaseURL: defaultBaseURL, Model: "model", APIKey: "fixture"},
		backend:   &Chat{},
		providers: providerCatalog{Selected: modelChoice{ProviderID: "p"}, Providers: []modelProvider{{ID: "p", Name: "Provider", Protocol: openAIChat, BaseURL: defaultBaseURL, APIKey: "fixture", Model: "model", Models: []string{"model", "other-model"}}}},
	}
	window := ui.NewTester(func(c *ui.Context) { ui.Column(c).Fill().Justify(ui.End).Children(func() { app.composer(c, nil) }) }, 390, 760)
	settle := func() {
		for range 90 {
			window.Advance(time.Second / 60)
		}
	}
	if err := window.Click(locales.ChooseChatModel()); err != nil {
		t.Fatal(err)
	}
	settle()
	if _, ok := window.Find("other-model"); !ok {
		t.Fatalf("model sheet is missing options: %v", window.Texts())
	}
	if err := window.Click("other-model"); err != nil {
		t.Fatal(err)
	}
	if app.currentModelChoice().Model != "other-model" {
		t.Fatalf("model selection was not applied: %v", app.providerError)
	}
	if err := window.Click(locales.ChooseChatModel()); err != nil {
		t.Fatal(err)
	}
	settle()
	window.FingerDown(5, 20)
	window.FingerUp(5, 20)
	settle()
	if _, ok := window.Find("Provider"); ok {
		t.Fatal("outside tap must close model sheet")
	}
	if err := window.Click(locales.AgentPermissions()); err != nil {
		t.Fatal(err)
	}
	settle()
	if err := window.Click(locales.ModeFull()); err != nil {
		t.Fatal(err)
	}
	if app.fullAccess == nil || app.draftAccess == localtools.Full {
		t.Fatal("sheet must retain full-access acknowledgement before enabling")
	}
}

func TestCompactComposerDoesNotOscillateAtWrapBoundary(t *testing.T) {
	for length := 16; length <= 27; length++ {
		app := &chatApp{backend: &Chat{}}
		app.draft.SetText(strings.Repeat("字", length))
		window := ui.NewTester(func(c *ui.Context) { app.composer(c, nil) }, 390, 700)
		for range 3 {
			window.Frame()
		}
		before, _ := window.Find(locales.Send())
		for range 10 {
			window.Frame()
			after, _ := window.Find(locales.Send())
			if before != after {
				t.Fatalf("send button oscillated near wrap boundary (%d chars): %v -> %v", length, before, after)
			}
		}
	}
}

func TestCompactLayoutRespectsSafeAreaAndKeyboard(t *testing.T) {
	app := &chatApp{backend: &Chat{}}
	window := ui.NewTester(app.view, 390, 780)
	window.SetInsets(55, 8, 315, 8)
	history, ok := window.Find(locales.ChatHistory())
	if !ok || history.Y < 55 || history.X < 8 {
		t.Fatalf("navigation overlaps the safe area: %+v", history)
	}
	send, ok := window.Find(locales.Send())
	if !ok || send.Y+send.H > 465 {
		t.Fatalf("send control overlaps the keyboard: %+v", send)
	}
}

func TestCompactHistoryLongPressOpensActions(t *testing.T) {
	app := &chatApp{backend: &Chat{}, compactHistory: true}
	app.newConversation("Conversation for touch test", "", "")
	window := ui.NewTester(app.view, 390, 720)
	item, ok := window.Find(locales.ChatActions())
	if !ok {
		t.Fatal("missing conversation in the drawer")
	}
	x, y := item.X-24, item.Y+item.H/2
	window.Frame()
	window.FingerDown(x, y)
	window.Advance(400 * time.Millisecond)
	window.Advance(200 * time.Millisecond)
	window.Frame()
	window.FingerUp(x, y)
	if !window.HasText(locales.ChatInfo()) || !app.compactHistory {
		t.Fatalf("holding a conversation should open its actions without closing the drawer: %v", window.Texts())
	}
}

func TestCompactWelcomeSuggestionsKeepTwoColumns(t *testing.T) {
	app := &chatApp{backend: &Chat{}}
	window := ui.NewTester(app.view, 320, 680)
	weight, weightVisible := window.Find(locales.WeightSuggestion())
	outfit, outfitVisible := window.Find(locales.OutfitSuggestion())
	travel, travelVisible := window.Find(locales.TravelSuggestion())
	route, routeVisible := window.Find(locales.RouteSuggestion())
	if !weightVisible || !outfitVisible || !travelVisible || !routeVisible ||
		weight.Y != outfit.Y || travel.Y != route.Y || travel.Y <= weight.Y ||
		weight.X != travel.X || outfit.X != route.X || route.X+route.W > 320 {
		t.Fatalf("suggestions should occupy a two-by-two grid: %v %v %v %v", weight, outfit, travel, route)
	}
}

func TestCompactComposerGrowsForWrappedDraft(t *testing.T) {
	app := &chatApp{backend: &Chat{}}
	window := ui.NewTester(func(c *ui.Context) { app.composer(c, nil) }, 320, 540)
	window.Frame()
	empty, _ := window.Find(locales.Message())
	app.draft.SetText("我今天要通勤，下班后还要和朋友吃饭。帮我用日常单品搭一套舒服、简单又得体的衣服。")
	for range 4 {
		window.Frame()
	}
	wrapped, ok := window.Find(locales.Message())
	if !ok || wrapped.H <= empty.H {
		t.Fatalf("a wrapped paragraph should grow instead of scrolling into the top inset: empty=%+v wrapped=%+v", empty, wrapped)
	}
	app.draft.SetText("")
	for range 4 {
		window.Frame()
	}
	cleared, _ := window.Find(locales.Message())
	if cleared.H != empty.H {
		t.Fatalf("clearing the draft should restore its single-line height: empty=%+v cleared=%+v", empty, cleared)
	}
}

func TestCompactConversationTitleAndExpandShareOneLine(t *testing.T) {
	const title = "我今天要通勤，下班后还要和朋友吃饭。帮我用日常单品搭一套舒服、简单又得体的衣服。"
	window := ui.NewTester(func(c *ui.Context) {
		(&chatApp{}).conversationInfoDialog(c, &conversation{ID: newID(), Title: title})
	}, 320, 780)
	window.Advance(time.Millisecond)
	text, ok := window.Find(title)
	button, shown := window.Find(locales.ChatTitleView())
	if !ok || !shown || button.X < text.X+text.W-1 || button.Y >= text.Y+text.H ||
		button.Y+button.H/2 > text.Y+text.H/2+1 || button.Y+button.H/2 < text.Y+text.H/2-1 {
		t.Fatalf("collapsed title and expand must share a row: title=%+v expand=%+v", text, button)
	}
}

func TestCompactSettingsBackDismissesMenuBeforeLeaving(t *testing.T) {
	app := &chatApp{backend: &Chat{}, language: settings.SystemLanguage}
	app.loadSettings()
	app.openSettings()
	app.settingsTab = settings.General
	window := ui.NewTester(app.view, 390, 780)
	if err := window.Click(locales.FollowSystem()); err != nil {
		t.Fatal(err)
	}
	window.Key(0, ui.KeyEscape)
	if !app.settingsOpen {
		t.Fatal("Back on an open selector must dismiss it before leaving settings")
	}
	window.Key(0, ui.KeyEscape)
	if app.settingsOpen {
		t.Fatal("Back from settings must return to the conversation")
	}
}
