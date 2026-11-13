package chat

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	timeline "github.com/ggdream/microlight/internal/activity"
	"github.com/ggdream/microlight/assets/locales"
	"github.com/ggdream/microlight/internal/localtools"
	"github.com/ggdream/microlight/pkg/diff"

	"github.com/egoist/mygo/ui"
)

func TestReasoningAndBodyStreamSeparately(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"reasoning_content\":\"Reasoning sample\"}}]}\n\n")
		fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"Body sample\"}}]}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	backend := newChat(modelSettings{BaseURL: server.URL, Model: "test-reasoning"})
	var received []replyDelta
	err := backend.stream(t.Context(), []message{{Role: "user", Content: "Test"}}, func(delta replyDelta) error {
		received = append(received, delta)
		return nil
	})
	if err != nil || len(received) != 2 ||
		received[0] != (replyDelta{reasoning: "Reasoning sample"}) ||
		received[1] != (replyDelta{text: "Body sample"}) {
		t.Fatalf("unexpected stream: %+v, %v", received, err)
	}
}

func TestReplyTimerAndReasoningDisclosure(t *testing.T) {
	reply := &turn{
		message: message{Role: "assistant"},
		State:   "pending", StartedAt: time.Now(), Activity: []timeline.Entry{{Text: "Reasoning sample"}},
	}
	chat := &conversation{ID: "chat", Turns: []*turn{reply}}
	app := &chatApp{}
	surface := ui.NewTester(func(c *ui.Context) { app.transcript(c, chat) }, 620, 420)
	if _, shown := surface.Find("Reasoning sample"); !shown {
		t.Fatal("reasoning is hidden before the body")
	}
	surface.Advance(2 * time.Second)
	surface.Frame()
	elapsed := locales.Elapsed(locales.DurationSeconds(2))
	timer, timerShown := surface.Find(elapsed)
	thinking, shown := surface.Find(locales.Thinking())
	reasoning, _ := surface.Find("Reasoning sample")
	if !timerShown || !shown || timer.Y >= reasoning.Y || thinking.Y <= reasoning.Y {
		t.Fatalf("timer should advance above the output and the thinking status should appear below it: %v", surface.Texts())
	}
	reply.appendDelta(replyDelta{preview: "Intermediate output"})
	surface.Frame()
	if _, shown := surface.Find(locales.Thinking()); shown {
		t.Fatal("thinking should be hidden while text is streaming")
	}
	tools := []timeline.Entry{{Tool: localtools.Write, Target: "example.go", CallID: "edit", Pending: true}}
	reply.appendDelta(replyDelta{messageDone: true, activity: &tools})
	surface.Frame()
	if _, shown := surface.Find(locales.WriteFile() + " example.go"); !shown {
		t.Fatal("the running tool row should remain visible")
	}
	if _, shown := surface.Find(locales.Thinking()); shown {
		t.Fatal("thinking should be hidden while a tool is running")
	}
	reply.appendDelta(replyDelta{toolFinished: "edit"})
	surface.Frame()
	if _, shown := surface.Find(locales.Thinking()); !shown {
		t.Fatal("thinking should resume after the tool returns")
	}
	reply.appendDelta(replyDelta{text: "Body sample"})
	surface.Frame()
	if _, shown := surface.Find("Reasoning sample"); shown {
		t.Fatal("reasoning did not collapse after the body started")
	}
	for _, status := range []string{locales.Thinking(), locales.Generating()} {
		if _, shown := surface.Find(status); shown {
			t.Fatalf("unexpected status %q below the answer", status)
		}
	}
	if err := surface.Click(locales.ReasoningAccessibility(elapsed)); err != nil {
		t.Fatal(err)
	}
	reply.appendDelta(replyDelta{text: " continues."})
	surface.Frame()
	if _, shown := surface.Find("Reasoning sample"); !shown {
		t.Fatal("new body content hid manually expanded reasoning")
	}
	reply.FinishedAt = reply.StartedAt.Add(3 * time.Second)
	reply.State = "done"
	surface = ui.NewTester(func(c *ui.Context) { app.transcript(c, chat) }, 620, 420)
	surface.Advance(5 * time.Second)
	if _, shown := surface.Find(locales.Elapsed(locales.DurationSeconds(3))); !shown {
		t.Fatalf("final duration did not freeze: %v", surface.Texts())
	}
	want := reply.FinishedAt.Local().Format("15:04")
	if _, shown := surface.Find(want); shown {
		t.Fatal("completion time is visible before hovering")
	}
	copyButton, found := surface.Find(locales.Copy())
	if !found {
		t.Fatal("copy button missing")
	}
	surface.Move(copyButton.X+copyButton.W/2, copyButton.Y+copyButton.H/2)
	timestamp, shown := surface.Find(want)
	if !shown || timestamp.X < copyButton.X+copyButton.W {
		t.Fatal("completion time should appear to the right of the reply actions")
	}
	surface.Move(0, 0)
	if _, shown := surface.Find(want); shown {
		t.Fatal("completion time is still visible after pointer exit")
	}
}

func TestAnsweredClarificationSitsBetweenReasoningSegments(t *testing.T) {
	const before = "先检查已知条件。"
	const after = "根据补充信息继续思考。"
	const question = "接下来怎么处理？"
	const answer = "直接按当前信息继续处理"
	offset := 3
	entry := &turn{
		message: message{Role: "assistant"},
		State:   "done", Activity: []timeline.Entry{
			{Text: before},
			{Tool: localtools.List, Target: "/tmp/project", Arguments: `{"path":"/tmp/project"}`},
			{Tool: "run_command", Target: "printf '%s' hello", Arguments: `{"command":"printf '%s' hello"}`},
			{Text: after},
		},
		Clarifications: []clarification{{Question: question, Answer: answer, ActivityOffset: &offset}},
	}
	app := &chatApp{}
	chat := &conversation{ID: "chat", Turns: []*turn{entry}}
	surface := ui.NewTester(func(c *ui.Context) { app.transcript(c, chat) }, 540, 360)
	if err := surface.Click(locales.Reasoning()); err != nil {
		t.Fatal(err)
	}
	first, firstShown := surface.Find(before)
	prompt, promptShown := surface.Find(locales.ClarificationRecord())
	last, lastShown := surface.Find(after)
	Label := locales.ListFolder() + " project"
	tool, toolShown := surface.Find(Label)
	if !firstShown || !toolShown || !promptShown || !lastShown || !(first.Y < tool.Y && tool.Y < prompt.Y && prompt.Y < last.Y) {
		t.Fatalf("unexpected reasoning timeline: %v", surface.Texts())
	}
	const commandText = "printf '%s' hello"
	if _, shown := surface.Find(commandText); shown {
		t.Fatal("tool details start expanded")
	}
	if err := surface.Click(Label); err != nil {
		t.Fatal(err)
	}
	if app.toolPanel.active == nil || app.toolPanel.active.index != 1 {
		t.Fatal("the folder target should open its preview")
	}
	if _, shown := surface.Find(entry.Activity[1].Details()); shown {
		t.Fatal("a non-command tool should not expand inline")
	}
	if err := surface.Click(locales.ToolArguments()); err != nil {
		t.Fatal(err)
	}
	details, detailsShown := surface.Find(commandText)
	prompt, _ = surface.Find(locales.ClarificationRecord())
	if !detailsShown || !(tool.Y < details.Y && details.Y < prompt.Y) {
		t.Fatalf("tool arguments did not expand in place: %v", surface.Texts())
	}
	if err := surface.Click(locales.ToolArguments()); err != nil {
		t.Fatal(err)
	}
	if _, shown := surface.Find(commandText); shown {
		t.Fatal("second click did not collapse the tool details")
	}
	prompt, _ = surface.Find(locales.ClarificationRecord())
	if _, shown := surface.Find(question); shown {
		t.Fatal("question starts expanded")
	}
	if _, shown := surface.Find(answer); shown {
		t.Fatal("answer starts expanded")
	}
	if err := surface.Click(locales.ClarificationRecord()); err != nil {
		t.Fatal(err)
	}
	detail, shown := surface.Find(answer)
	original, originalShown := surface.Find(question)
	last, _ = surface.Find(after)
	if !shown || !originalShown || !(prompt.Y < original.Y && original.Y < detail.Y && detail.Y < last.Y) {
		t.Fatalf("answer did not expand in place: %v", surface.Texts())
	}
	if err := surface.Click(locales.ClarificationRecord()); err != nil {
		t.Fatal(err)
	}
	if _, shown := surface.Find(answer); shown {
		t.Fatal("second click did not collapse the answer")
	}
}

func TestToolTargetOpensPreviewAndContextMenu(t *testing.T) {
	entry := &turn{message: message{Role: "assistant"}, State: "done", Activity: []timeline.Entry{
		{Tool: localtools.Write, Target: "/tmp/example.go", Diff: diff.Compare("previous line", "updated line")},
		{Tool: localtools.Write, Target: "/tmp/internal/extra.go", Diff: diff.Compare("", "extra content")},
		{Tool: localtools.Write, Target: "/tmp/internal/denied.go"},
	}}
	chat := &conversation{ID: "chat", Title: "Test", Turns: []*turn{entry}, workspace: "/tmp"}
	app := &chatApp{backend: &Chat{}, history: history{Selected: chat.ID, Chats: []*conversation{chat}}}
	surface := ui.NewTester(app.view, 1024, 650)
	if err := surface.Click(locales.Reasoning()); err != nil {
		t.Fatal(err)
	}
	label := locales.WriteFile() + " example.go"
	if err := surface.Click(label); err != nil {
		t.Fatal(err)
	}
	if _, found := surface.Find("previous line"); !found {
		t.Fatalf("removed line missing from preview: %v", surface.Texts())
	}
	if _, found := surface.Find("updated line"); !found {
		t.Fatalf("added line missing from preview: %v", surface.Texts())
	}
	if err := surface.Click(locales.ClosePreview()); err != nil {
		t.Fatal(err)
	}
	if err := surface.RightClick(label); err != nil {
		t.Fatal(err)
	}
	if err := surface.Click(locales.OpenPreview()); err != nil {
		t.Fatal(err)
	}
	if _, found := surface.Find("updated line"); !found {
		t.Fatal("context menu did not open the preview")
	}
	if err := surface.Click(locales.Reasoning()); err != nil {
		t.Fatal(err)
	}
	if _, found := surface.Find("denied.go"); found {
		t.Fatal("an unsuccessful write appeared in the change tree")
	}
	if err := surface.Click("internal"); err != nil {
		t.Fatal(err)
	}
	if _, found := surface.Find("extra.go"); found {
		t.Fatal("collapsing the folder left its file visible")
	}
	if err := surface.Click("internal"); err != nil {
		t.Fatal(err)
	}
	if err := surface.Click("extra.go"); err != nil {
		t.Fatal(err)
	}
	if _, found := surface.Find("extra content"); !found {
		t.Fatal("selecting a file in the tree did not open its tab")
	}
	if err := surface.Click(locales.CloseFileTab("extra.go")); err != nil {
		t.Fatal(err)
	}
	if _, found := surface.Find("updated line"); !found {
		t.Fatal("closing one file tab did not restore the other open tab")
	}
	if err := surface.Click(locales.AddAttachment()); err != nil {
		t.Fatal(err)
	}
	for _, item := range []string{locales.UploadFile(), locales.PlanMode()} {
		if _, found := surface.Find(item); !found {
			t.Errorf("composer menu missing %q", item)
		}
	}
}
