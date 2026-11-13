package chat

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/ggdream/microlight/assets/locales"
	"github.com/ggdream/microlight/internal/linkpreview"
	"github.com/ggdream/microlight/pkg/markup"
)

func TestLinkPreviewHoverAndOpen(t *testing.T) {
	const dest = "https://example.org/guide#install"
	app := &chatApp{md: markup.New(ink)}
	app.md.Preview = app.links.offer
	const title = "Installation guide"
	app.links.results = map[previewRequest]previewResult{
		{url: "https://example.org/guide"}: {
			metadata: linkpreview.Metadata{Title: title, Description: "Getting started"},
			until:    time.Now().Add(time.Minute),
		},
	}
	chat := &conversation{ID: "test-chat", Turns: []*turn{{
		message: message{Role: "assistant", Content: "[Visit guide](" + dest + ")"}, State: "done",
	}}}
	window := ui.NewTester(func(c *ui.Context) {
		root := ui.Column(c).Fill().Padding(155, 20, 0).Children(func() {
			app.transcript(c, chat)
		})
		app.linkPreview(c, root)
	}, 480, 420)
	link, ok := window.Find("Visit guide")
	if !ok {
		t.Fatalf("could not find link in %v", window.Texts())
	}
	window.Move(link.X+16, link.Y+link.H/2)
	if window.HasText(title) {
		t.Fatal("preview appeared before the hover delay")
	}
	window.Advance(800 * time.Millisecond)
	open, ok := window.Find(locales.LinkPreviewOpen())
	if !ok || !window.HasText(title) || !window.HasText("Getting started") {
		t.Fatalf("missing preview content: %v", window.Texts())
	}
	window.Move(470, 40)
	window.Advance(500 * time.Millisecond)
	if window.HasText(title) {
		t.Fatal("preview stayed open after the pointer left both the link and the card")
	}
	window.Move(link.X+16, link.Y+link.H/2)
	window.Advance(800 * time.Millisecond)
	open, ok = window.Find(locales.LinkPreviewOpen())
	if !ok {
		t.Fatal("preview did not reopen when the link was hovered again")
	}

	window.Move(open.X+open.W/2, open.Y+open.H/2)
	window.Advance(500 * time.Millisecond)
	if !window.HasText(title) {
		t.Fatal("preview disappeared while the pointer was inside the card")
	}
	window.ClickAt(open.X+open.W/2, open.Y+open.H/2)
	if urls := window.OpenedURLs(); len(urls) != 1 || urls[0] != dest {
		t.Fatalf("expected the original URL including its fragment, opened %v", urls)
	}
}
