package chat

import (
	"testing"
	"time"

	"github.com/ggdream/microlight/assets/locales"
	"github.com/ggdream/microlight/internal/attachment"
	"github.com/ggdream/microlight/internal/localtools"

	"github.com/egoist/mygo/ui"
)

func TestConversationMenusAndArchiveRestore(t *testing.T) {
	store, err := openLocalStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	app := &chatApp{store: store, backend: &Chat{}}
	first := app.newConversation("First conversation", localtools.Ask, "")
	app.save(first)
	second := app.newConversation("Second conversation", localtools.Ask, "")
	app.save(second)
	window := ui.NewTester(app.view, 900, 640)
	click := func(label string) {
		t.Helper()
		if err := window.Click(label); err != nil {
			t.Fatal(err)
		}
	}
	rightClick := func(label string) {
		t.Helper()
		if err := window.RightClick(label); err != nil {
			t.Fatal(err)
		}
	}
	find := func(label string) ui.Rect {
		t.Helper()
		r, ok := window.Find(label)
		if !ok {
			t.Fatalf("missing %q", label)
		}
		return r
	}
	if _, ok := window.Find(locales.ChatActions()); ok {
		t.Fatal("the menu button should be hidden before hover")
	}
	row := find(first.Title)
	first.run = &replyRun{}
	window.Frame()
	window.Move(row.X+row.W/2, row.Y+row.H/2)
	find(locales.Thinking())
	if _, visible := window.Find(locales.ChatActions()); visible {
		t.Fatal("the running indicator should replace the menu button on hover")
	}
	rightClick(first.Title)
	find(locales.RenameChat())
	click(locales.ChatInfo())
	find(locales.ChatTitle())
	find(locales.ChatCreated())
	find(locales.ChatLastActivity())
	find(locales.ChatMessageCount())
	find(locales.WorkspaceUnspecified())
	if _, shown := window.Find(locales.ChatTitleView()); shown {
		t.Fatal("a title that fits should not offer an expand action")
	}
	click(locales.CloseChatInfo())
	shortTitle := first.Title
	first.Title = "A much longer conversation title that does not fit on one line in the information dialog"
	window.Frame()
	rightClick(first.Title)
	click(locales.ChatInfo())
	window.Advance(time.Millisecond)
	click(locales.ChatTitleView())
	find(locales.ChatTitleCollapse())
	click(locales.ChatTitleCollapse())
	find(locales.ChatTitleView())
	click(locales.CloseChatInfo())
	if app.chatAction != nil || app.selected() != second {
		t.Fatal("viewing another conversation’s details changed the selection")
	}

	first.Title = shortTitle
	first.run = nil
	window.Frame()
	window.Move(row.X+row.W/2, row.Y+row.H/2)
	click(locales.ChatActions())
	for _, label := range []string{locales.RenameChat(), locales.PinChat(), locales.ArchiveChat(), locales.DeleteChat()} {
		find(label)
	}
	click(locales.PinChat())
	if app.sidebarChats()[0] != first || app.selected() != second {
		t.Fatal("pin must reorder the sidebar without changing the selected conversation")
	}
	rightClick(first.Title)
	click(locales.RenameChat())
	window.Key(ui.Cmd, ui.KeyA)
	window.Type("Renamed conversation")
	click(locales.SaveChatTitle())
	if first.Title != "Renamed conversation" {
		t.Fatalf("rename did not save: %q", first.Title)
	}

	rightClick(first.Title)
	click(locales.ArchiveChat())
	if _, ok := window.Find(first.Title); ok {
		t.Fatal("archived conversation is still in the sidebar")
	}
	saved, err := store.loadHistory()
	if err != nil || len(saved.Chats) != 2 || !saved.Chats[1].archived ||
		!saved.Chats[1].pinned || saved.Chats[1].Title != first.Title {
		t.Fatalf("archive, pin and title must survive reloading: %+v, %v", saved, err)
	}
	click(locales.Settings())
	click(locales.ArchivedChats())
	find(first.Title)
	click(locales.RestoreChat())
	find(locales.NoArchivedChats())
	click(locales.BackToChat())
	find(first.Title)
	if app.sidebarChats()[0] != first || app.selected() != second {
		t.Fatal("restore must put the conversation back with its pin intact")
	}

	rightClick(first.Title)
	click(locales.DeleteChat())
	click(locales.Cancel())
	find(first.Title)
	rightClick(first.Title)
	click(locales.DeleteChat())
	find(locales.DeleteChatTitle())
	click(locales.DeleteChat())
	if _, ok := window.Find(first.Title); ok {
		t.Fatal("deleted conversation is still in the sidebar")
	}
}

func TestDeleteConversationRemovesOwnedData(t *testing.T) {
	store, err := openLocalStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	app := &chatApp{store: store}
	other := app.newConversation("Keep", localtools.Ask, "")
	app.save(other)
	chat := app.newConversation("Delete", localtools.Ask, "")
	legacy := chat.ID + ":old"
	previous := newID()
	chat.sessionID = previous
	app.save(chat)
	chat.sessionID = newID()
	chat.Turns = []*turn{
		{message: message{Role: "user", Content: "hello", Attachments: []*attachment.File{{Name: "example.png", MIME: "image/png", Data: []byte{1, 2, 3}}}}},
		{State: "waiting", Clarifications: []clarification{{CheckpointID: "legacy-checkpoint"}}},
	}
	app.save(chat)
	if err := store.saveDraft(chat.ID, "draft", true, nil, false); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{legacy, previous, chat.sessionID, "unrelated"} {
		if _, err := store.Exec("INSERT INTO adk_session_events (session_id, event_id, kind, payload) VALUES (?, '1', '', X'00')", id); err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []string{legacy + ":checkpoint", "legacy-checkpoint", "unrelated"} {
		if err := store.Set(t.Context(), id, []byte("checkpoint")); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.Exec("INSERT INTO adk_checkpoints (id, payload, session_id) VALUES (?, X'00', ?)", newID(), previous); err != nil {
		t.Fatal(err)
	}
	if !app.deleteChat(chat) {
		t.Fatal("delete failed:", app.historyErr)
	}
	for query, want := range map[string]int{
		"SELECT COUNT(*) FROM conversations":                                     1,
		"SELECT COUNT(*) FROM turns":                                             0,
		"SELECT COUNT(*) FROM turn_images":                                       0,
		"SELECT COUNT(*) FROM adk_session_events WHERE session_id = 'unrelated'": 1,
		"SELECT COUNT(*) FROM adk_session_events":                                1,
		"SELECT COUNT(*) FROM adk_checkpoints WHERE id = 'unrelated'":            1,
		"SELECT COUNT(*) FROM adk_checkpoints":                                   1,
		"SELECT COUNT(*) FROM app_data WHERE key LIKE 'draft:%'":                 0,
	} {
		var n int
		if err := store.QueryRow(query).Scan(&n); err != nil || n != want {
			t.Errorf("%s: got %d, want %d, error %v", query, n, want, err)
		}
	}
	h, err := store.loadHistory()
	if err != nil || h.Selected != "" || len(h.Chats) != 1 || h.Chats[0].ID != other.ID {
		t.Fatalf("unexpected remaining history: %+v, %v", h, err)
	}
}

func TestClippedSidebarTitleScrollsOnlyOnHover(t *testing.T) {
	const title = "A conversation title that is much longer than the available sidebar width"
	hovered := false
	var x, width float32
	window := ui.NewTester(func(c *ui.Context) {
		ui.Row(c).Width(125).Height(34).AlignItems(ui.Center).Children(func() {
			track := sidebarTitle(c.Key("title"), title, ink, hovered)
			x, _ = track.LocalToWindow(0, 0)
			width = track.LayoutBounds().W
		})
	}, 150, 45)
	titlePosition := func() float32 {
		t.Helper()
		// Element geometry reflects the previous frame.
		window.Frame()
		if width <= 125 {
			t.Fatalf("expected the title to overflow, got width %v", width)
		}
		return x
	}
	window.Advance(time.Millisecond)
	idle := titlePosition()
	hovered = true
	window.Frame()
	window.Advance(time.Second)
	if position := titlePosition(); position >= idle {
		t.Fatalf("the clipped title did not scroll on hover: start %v, now %v", idle, position)
	}
	hovered = false
	window.Frame()
	if position := titlePosition(); position != idle {
		t.Fatalf("the title did not return to its start after hover: got %v, want %v", position, idle)
	}
	window.SetPreferences(ui.Preferences{ReduceMotion: true})
	hovered = true
	window.Frame()
	window.Advance(time.Second)
	if position := titlePosition(); position != idle {
		t.Fatalf("the title scrolled with reduced motion enabled: got %v, want %v", position, idle)
	}
}

func TestConversationTimes(t *testing.T) {
	createdAt := time.UnixMilli(1700000000000)
	first := createdAt.Add(time.Minute)
	last := first.Add(time.Hour)
	chat := &conversation{
		ID: "018bcfe5-6800-7000-8000-000000000000",
		Turns: []*turn{
			{StartedAt: first},
			{StartedAt: first.Add(time.Minute), FinishedAt: last},
			{StartedAt: first.Add(2 * time.Minute)},
		},
	}
	created, latest, known := conversationTimes(chat)
	if !created.Equal(createdAt) || !latest.Equal(last) || !known {
		t.Fatalf("unexpected UUIDv7 conversation timestamps: %v, %v, %v", created, latest, known)
	}
	chat.ID = "legacy-id"
	created, latest, known = conversationTimes(chat)
	if !created.Equal(first) || !latest.Equal(last) || known {
		t.Fatalf("unexpected legacy conversation timestamps: %v, %v, %v", created, latest, known)
	}
}
