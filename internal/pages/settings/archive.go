package settings

import (
	"github.com/ggdream/microlight/assets/locales"
	asseticon "github.com/ggdream/microlight/internal/icon"
	"github.com/ggdream/microlight/internal/theme"
	"github.com/ggdream/microlight/pkg/viewport"

	"github.com/egoist/mygo/ui"
	"github.com/ggdream/microlight/pkg/elastic"
	"github.com/ggdream/microlight/pkg/material3"
)

type ArchivedChat struct{ ID, Title string }

func ArchivedChats(c *ui.Context, icons asseticon.Set, chats []ArchivedChat, historyError string, onRestore func(string)) {
	scroll, offset := elastic.Scroll(c)
	scroll.Grow(1).MinHeight(0).Children(func() {
		offset.Apply(ui.Column(c)).FillWidth().AlignItems(ui.Center).Padding(settingsLayout(c).Y(settingsLayout(c).Density(26, 14)), settingsLayout(c).Gutter(22)).Children(func() {
			ui.Column(c).FillWidth().MaxWidth(610).Gap(settingGap(c, 14)).Children(func() {
				ui.Column(c).Gap(settingGap(c, 5)).Children(func() {
					ui.Text(c, locales.ArchivedChats()).FontSize(viewport.Font(c, 29)).FontWeight(680)
					ui.Text(c, locales.ArchivedChatsHint()).FontSize(viewport.Font(c, 13)).TextColor(theme.Muted)
				})
				if historyError != "" {
					ui.Text(c, historyError).FontSize(viewport.Font(c, 12)).TextColor(theme.ErrorFG)
				}
				material3.CardStyle(ui.Column(c).FillWidth(), material3.ElevatedCard).Children(func() {
					empty := true
					for _, chat := range chats {
						row := ui.Row(c.Key(chat.ID)).FillWidth().MinHeight(settingsLayout(c).Target(50)).Padding(settingY(c, 8), settingX(c, 12)).Gap(settingGap(c, 10)).AlignItems(ui.Center)
						if !empty {
							material3.EdgeDivider(row, material3.TopEdge)
						}
						empty = false
						row.Children(func() {
							ui.Icon(c, icons.SVG("archive")).Size(16, 16).Shrink(0).TextColor(theme.Muted)
							ui.Text(c, chat.Title).Grow(1).MinWidth(0).SingleLine().Ellipsis("…").Tooltip(chat.Title).FontSize(viewport.Font(c, 13))
							label := locales.RestoreChat()
							restore := material3.ButtonStyle(theme.PointerButton(c.Key("restore"), false).Label(label).Height(settingsLayout(c).Target(30)).PaddingX(settingX(c, 9)).
								Shrink(0).Gap(settingGap(c, 5)).AlignItems(ui.Center), material3.ElevatedButton, false)
							if restore.Hovered() {
								restore.Background(theme.Hover)
							}
							restore.Children(func() {
								ui.Icon(c, icons.SVG("archive_arrow_back")).Size(14, 14).Shrink(0).TextColor(theme.Accent)
								ui.Text(c, label).FontSize(viewport.Font(c, 12)).FontWeight(610).TextColor(theme.Accent)
							}).OnClick(func() { onRestore(chat.ID) })
						})
					}
					if empty {
						ui.Text(c, locales.NoArchivedChats()).Padding(settingY(c, settingsLayout(c).Density(20, 12)), settingX(c, 12)).FontSize(viewport.Font(c, 13)).TextColor(theme.Muted)
					}
				})
			})
		})
	})
}
