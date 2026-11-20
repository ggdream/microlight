package chat

import (
	"github.com/ggdream/microlight/assets/locales"
	"github.com/ggdream/microlight/internal/localtools"
	"github.com/ggdream/microlight/pkg/viewport"

	"github.com/egoist/mygo/ui"
	"github.com/ggdream/microlight/pkg/elastic"
	"github.com/ggdream/microlight/pkg/material3"
)

func (app *chatApp) clarificationCards(c *ui.Context, chat *conversation, reply *turn) {
	for index, question := range reply.Clarifications {
		if question.Answer != "" {
			continue
		}
		material3.CardStyle(ui.Column(c.Key(index)).FillWidth().Padding(windowLayout(c).Density(12, 8)).Gap(8),
			material3.ElevatedCard).Radius(material3.ShapeLarge).Children(func() {
			waiting := reply.State == "waiting" && index == len(reply.Clarifications)-1
			if waiting && question.Approval == nil {
				app.clarificationForm(c, chat, reply, question)
				return
			}
			ui.Text(c, question.displayTitle()).FontSize(viewport.Font(c, 14)).FontWeight(620).Selectable()
			if question.Approval != nil {
				app.approvalControls(c, chat, reply, question.Approval, waiting)
			}
		})
	}
}

func (app *chatApp) approvalControls(c *ui.Context, chat *conversation, reply *turn, approval *localtools.Approval, waiting bool) {
	layout := windowLayout(c)
	ui.Text(c, approval.Target).FontSize(viewport.Font(c, 12)).TextColor(muted).FillWidth().Selectable()
	if !waiting {
		return
	}
	if approval.Preview != "" {
		scroll, offset := elastic.Scroll(c)
		scroll.FillWidth().MaxHeight(layout.Y(140)).Background(side).Radius(7).Padding(layout.Y(8), layout.X(8)).Children(func() {
			offset.Apply(ui.Text(c, approval.Preview)).FontSize(viewport.Font(c, 12)).TextColor(ink).FillWidth().Selectable()
		})
	}
	disabled := chat.isRunning()
	ui.Row(c).FillWidth().Wrap().Justify(ui.End).AlignItems(ui.Center).Gap(layout.X(layout.Density(8, 5))).Children(func() {
		rejectLabel := locales.RejectOperation()
		reject := material3.Button(c, rejectLabel, material3.TextButton, disabled).Height(layout.Target(replyControlHeight)).MaxWidthPercent(100).PaddingX(layout.X(layout.Density(12, 8)))
		reject.Children(func() {
			ui.Text(c, rejectLabel).FontSize(viewport.Font(c, 12)).TextColor(muted)
		}).OnClick(func() { app.answerClarification(chat, reply, localtools.Deny) })

		allowLabel := locales.ApproveOnce()
		allow := material3.Button(c, allowLabel, material3.FilledButton, disabled).Height(layout.Target(replyControlHeight)).MaxWidthPercent(100).PaddingX(layout.X(layout.Density(12, 8)))
		allow.Children(func() {
			ui.Text(c, allowLabel).FontSize(viewport.Font(c, 12)).FontWeight(620).TextColor(material3.Foreground(material3.FilledButton, disabled))
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
	button := material3.Button(c, label, material3.ElevatedButton, disabled).Height(windowLayout(c).Target(32)).PaddingX(12)
	button.Children(func() {
		ui.Text(c, label).FontSize(viewport.Font(c, 12)).TextColor(accent)
	}).OnClick(action)
}
