package chat

import (
	"github.com/ggdream/microlight/assets/locales"
	"github.com/ggdream/microlight/internal/localtools"

	"github.com/egoist/mygo/ui"
	"github.com/ggdream/microlight/pkg/elastic"
)

func (app *chatApp) clarificationCards(c *ui.Context, chat *conversation, reply *turn) {
	for index, question := range reply.Clarifications {
		if question.Answer != "" {
			continue
		}
		ui.Column(c.Key(index)).FillWidth().Padding(12).Gap(8).
			Background(white).Border(1, border).Radius(10).Children(func() {
			waiting := reply.State == "waiting" && index == len(reply.Clarifications)-1
			if waiting && question.Approval == nil {
				app.clarificationForm(c, chat, reply, question)
				if app.active != nil {
					ui.Text(c, locales.OtherChatGenerating()).FontSize(11).TextColor(muted)
				}
				return
			}
			ui.Text(c, question.displayTitle()).FontSize(14).FontWeight(620).Selectable()
			if question.Approval != nil {
				app.approvalControls(c, chat, reply, question.Approval, waiting)
			}
		})
	}
}

func (app *chatApp) approvalControls(c *ui.Context, chat *conversation, reply *turn, approval *localtools.Approval, waiting bool) {
	ui.Text(c, approval.Target).FontSize(12).TextColor(muted).FillWidth().Selectable()
	if !waiting {
		return
	}
	if approval.Preview != "" {
		scroll, offset := elastic.Scroll(c)
		scroll.FillWidth().MaxHeight(140).Background(side).Radius(7).Padding(8).Children(func() {
			offset.Apply(ui.Text(c, approval.Preview)).FontSize(12).TextColor(ink).FillWidth().Selectable()
		})
	}
	disabled := app.active != nil
	ui.Row(c).FillWidth().Justify(ui.End).AlignItems(ui.Center).Gap(8).Children(func() {
		rejectLabel := locales.RejectOperation()
		reject := pointerButton(c, disabled).Label(rejectLabel).Height(replyControlHeight).
			PaddingX(12).Radius(replyControlRadius).AlignItems(ui.Center)
		if reject.Hovered() && !disabled {
			reject.Background(side)
		}
		reject.Children(func() {
			ui.Text(c, rejectLabel).FontSize(12).TextColor(muted)
		}).OnClick(func() { app.answerClarification(chat, reply, localtools.Deny) })

		allowLabel := locales.ApproveOnce()
		allow := pointerButton(c, disabled).Label(allowLabel).Height(replyControlHeight).
			PaddingX(12).Radius(replyControlRadius).AlignItems(ui.Center)
		background, foreground := tint, accent
		if disabled {
			background, foreground = side, muted
		} else if allow.Hovered() {
			background, foreground = accent, white
		}
		allow.Background(background).Children(func() {
			ui.Text(c, allowLabel).FontSize(12).FontWeight(620).TextColor(foreground)
		}).OnClick(func() { app.answerClarification(chat, reply, localtools.AllowOnce) })

		stopButton(c).OnClick(func() { app.stopReply(chat, reply) })
	})
}

func (q clarification) displayTitle() string {
	if q.Approval != nil {
		switch q.Question {
		case "运行命令":
			return locales.RunCommand()
		case "读取文件":
			return locales.ReadFile()
		case "查看目录":
			return locales.ListFolder()
		case "覆盖文件":
			return locales.OverwriteFile()
		case "创建文件":
			return locales.CreateFile()
		case "访问网页":
			return locales.VisitURL()
		}
	}
	return clarificationQuestionText(q.Question)
}

func (q clarification) displayAnswer() string {
	if q.Approval != nil {
		switch q.Answer {
		case localtools.AllowOnce:
			return locales.ApproveOnce()
		case localtools.Deny:
			return locales.RejectOperation()
		}
	}
	return q.Answer
}

func clarificationButton(c *ui.Context, label string, disabled bool, action func()) {
	button := pointerButton(c, disabled).Label(label).Padding(6, 9).Radius(6).
		Border(1, border).Background(side)
	if button.Hovered() {
		button.Background(tint)
	}
	button.Children(func() {
		ui.Text(c, label).FontSize(12).TextColor(accent)
	}).OnClick(action)
}
