package chat

import (
	"strconv"
	"time"

	"github.com/cloudwego/eino/adk/backgroundtask"
	"github.com/egoist/mygo/ui"
	"github.com/ggdream/microlight/assets/locales"
	"github.com/ggdream/microlight/internal/background"
	"github.com/ggdream/microlight/pkg/elastic"
)

func (app *chatApp) backgroundTaskIndicator(c *ui.Context) {
	rows := app.backgroundRows
	if len(rows) == 0 {
		return
	}
	label := locales.BackgroundTasks()
	button := pointerButton(c.Key("indicator"), false).Label(label).Tooltip(label).
		Height(30).Shrink(0).PaddingX(8).AlignItems(ui.Center).Gap(6).Radius(8).Expanded(app.backgroundOpen)
	if button.Hovered() || app.backgroundOpen {
		button.Background(tint)
	}
	button.Children(func() {
		icon(c, "window_console", accent, 16)
		ui.Text(c, strconv.Itoa(len(rows))).FontSize(12).TextColor(accent)
	}).OnClick(func() {
		app.backgroundOpen = !app.backgroundOpen
		app.backgroundError = ""
	})

	ui.Popover(c.Key("running-tasks"), button, &app.backgroundOpen, func() {
		ui.Column(c).Width(340).Padding(12).Gap(8).Children(func() {
			ui.Text(c, label).FontSize(13).FontWeight(640)
			if app.backgroundError != "" {
				ui.Text(c, app.backgroundError).FontSize(12).TextColor(errorFG)
			}
			scroll, offset := elastic.Scroll(c)
			scroll.FillWidth().MaxHeight(260).Children(func() {
				offset.Apply(ui.Column(c)).FillWidth().Gap(3).Children(func() {
					for _, task := range rows {
						app.backgroundTaskRow(c.Key(task.Spec.ID), task)
					}
				})
			})
		})
	})
}

func (app *chatApp) backgroundTaskRow(c *ui.Context, row background.Row) {
	label := locales.TaskRunning()
	switch {
	case row.Status == backgroundtask.StatusPending:
		label = locales.TaskQueued()
	case row.CancelRequestedAt != nil:
		label = locales.TaskStopping()
	}
	id := row.Spec.ID
	ui.Row(c).FillWidth().Padding(5, 4).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
			ui.Text(c, row.Spec.Description).SingleLine().Ellipsis("…").Tooltip(row.Spec.Description).FontSize(13).FontWeight(600)
			ui.Row(c).FillWidth().Gap(6).Children(func() {
				ui.Text(c, label).FontSize(11).TextColor(accent).Shrink(0)
				if app.hasWorkspaceOverlap(row.Directory, nil, id) {
					ui.Text(c, locales.WorkspaceConcurrent()).SingleLine().Ellipsis("…").MinWidth(0).FontSize(11).TextColor(errorFG)
				} else if chat := app.visibleChat(row.Conversation); chat != nil {
					ui.Text(c, chat.Title).SingleLine().Ellipsis("…").MinWidth(0).FontSize(11).TextColor(muted)
				}
			})
		})
		if app.visibleChat(row.Conversation) != nil {
			taskIconButton(c.Key("conversation"), locales.OpenConversationTask(), "chat", func() {
				app.backgroundOpen = false
				app.selectChat(row.Conversation)
			})
		}
		if row.CancelRequestedAt == nil {
			taskIconButton(c.Key("stop"), locales.TaskStop(), "stop", func() {
				go func() {
					err := app.tasks.Stop(app.appContext(), id)
					app.win.Update(func() {
						app.backgroundRefresh = time.Time{}
						if err != nil {
							app.backgroundError = locales.TaskStopFailed()
						}
					})
				}()
			})
		}
	})
}

func taskIconButton(c *ui.Context, label, glyph string, action func()) {
	button := pointerButton(c, false).Label(label).Tooltip(label).
		Size(28, 28).Shrink(0).Radius(6).AlignItems(ui.Center).Justify(ui.Center)
	if button.Hovered() {
		button.Background(hover)
	}
	button.Children(func() { icon(c, glyph, muted, 15) }).OnClick(action)
}

func (app *chatApp) syncBackgroundTasks(c *ui.Context) {
	now := c.Now()
	if app.backgroundRefresh.IsZero() || !now.Before(app.backgroundRefresh) {
		app.backgroundRows = app.tasks.ActiveRows()
		app.backgroundRefresh = now.Add(time.Second)
	}
	if len(app.backgroundRows) == 0 {
		app.backgroundOpen = false
		for _, chat := range app.history.Chats {
			if chat.isRunning() {
				c.After(app.backgroundRefresh.Sub(now))
				break
			}
		}
	} else {
		c.After(app.backgroundRefresh.Sub(now))
	}
}
