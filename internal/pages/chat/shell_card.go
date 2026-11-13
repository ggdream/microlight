package chat

import (
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/ggdream/microlight/assets/locales"
	timeline "github.com/ggdream/microlight/internal/activity"
	"github.com/ggdream/microlight/pkg/elastic"
)

func shellCard(c *ui.Context, activity timeline.Entry) {
	command, input := activity.CommandText(), activity.CommandInput()
	prompt := "$ "
	if command == "" {
		command = activity.Details()
		prompt = ""
	}
	var output string
	if activity.Output != nil {
		output = *activity.Output
	}
	card := ui.Column(c).FillWidth().Gap(6).Padding(8, 11, 7).Radius(10).Border(1, border).
		Background(ui.Hex("#F0F2F1")).Clip()
	copied := ui.Local(card, "copied-until", func() time.Time { return time.Time{} })
	card.Children(func() {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Justify(ui.SpaceBetween).Children(func() {
			ui.Text(c, "Shell").FontSize(12).FontWeight(560).TextColor(muted)
			label, glyph := locales.Copy(), "copy"
			if remaining := copied.Sub(c.Now()); remaining > 0 {
				label, glyph = locales.Copied(), "check"
				c.After(remaining)
			}
			messageIcon(c.Key("copy-command"), label, glyph, command == "" && input == "" && output == "", func() {
				var parts []string
				for _, text := range []string{command, input, output} {
					if text != "" {
						parts = append(parts, text)
					}
				}
				c.WriteClipboard(strings.Join(parts, "\n"))
				*copied = c.Now().Add(1500 * time.Millisecond)
			})
		})
		scroll, offset := elastic.Scroll(c.Key("shell-transcript"))
		var transcript ui.Element
		scroll.FillWidth().MaxHeight(204).Children(func() {
			transcript = offset.Apply(ui.Column(c)).FillWidth().Shrink(0).Gap(7).Padding(2, 0, 4).Children(func() {
				if command != "" {
					shellCardText(c, prompt+command, ink)
				}
				if input != "" {
					ui.Column(c).FillWidth().Gap(3).Children(func() {
						ui.Text(c, locales.ShellInput()).FontSize(11).TextColor(muted)
						shellCardText(c, input, muted)
					})
				}
				if output != "" {
					shellCardText(c, strings.TrimSuffix(output, "\n"), muted)
				}
			})
		})
		viewport, content := scroll.Bounds(), transcript.Bounds()
		fadeTop, fadeBottom := content.Y < viewport.Y-1, content.Y+content.H > viewport.Y+viewport.H+1
		if fadeTop || fadeBottom {
			visible := ui.RGB(0, 0, 0)
			top, bottom := visible, visible
			if fadeTop {
				top = ui.Transparent
			}
			if fadeBottom {
				bottom = ui.Transparent
			}
			scroll.MaskLinearGradient(ui.LinearGradient{Angle: 180, Stops: []ui.GradientStop{
				{At: 0, Color: top}, {At: .065, Color: visible}, {At: .935, Color: visible}, {At: 1, Color: bottom},
			}})
		}
		label, glyph, color := shellResultLabel(activity)
		if label != "" {
			ui.Row(c).FillWidth().Justify(ui.End).AlignItems(ui.Center).Gap(4).Children(func() {
				if glyph != "" {
					icon(c, glyph, color, 13)
				}
				ui.Text(c, label).FontSize(11).TextColor(color)
			})
		}
	})
}

func shellCardText(c *ui.Context, text string, color ui.Color) {
	ui.Text(c, text).FillWidth().Font("monospace").FontSize(12).TextColor(color).Selectable()
}

func shellResultLabel(activity timeline.Entry) (string, string, ui.Color) {
	result := activity.CommandResult
	if result == nil {
		return "", "", muted
	}
	var label, glyph string
	color := muted
	switch {
	case result.TimedOut:
		label, color = locales.ShellTimedOut(), errorFG
	case result.Failed:
		label, color = locales.ShellFailed(), errorFG
	case result.ExitCode == nil:
	case *result.ExitCode == 0:
		label, glyph = locales.ShellSuccess(), "check"
	default:
		label, color = locales.ShellExitCode(strconv.Itoa(*result.ExitCode)), errorFG
	}
	if result.Truncated {
		if label != "" {
			label += " · "
		}
		label += locales.ShellOutputTruncated()
	}
	return label, glyph, color
}
