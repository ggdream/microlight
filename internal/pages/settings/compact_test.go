package settings

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"
	"github.com/ggdream/microlight/assets/locales"
	asseticon "github.com/ggdream/microlight/internal/icon"
	"github.com/ggdream/microlight/internal/mcptools"
)

func TestCompactSettingsSelectScrollsToLastOption(t *testing.T) {
	selected := 0
	options := []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	label := func(i int) string { return fmt.Sprintf("Provider %d", i+1) }
	window := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Fill().Children(func() {
			ui.Box(c).Height(190).Shrink(0)
			settingsSelect(c, asseticon.Set{}, "Provider", &selected, options, label, func() {})
		})
	}, 320, 460)
	window.SetInsets(25, 8, 95, 8)
	if err := window.Click("Provider 1"); err != nil {
		t.Fatal(err)
	}
	first, ok := window.Find("Provider 2")
	if !ok || first.H < 44 {
		t.Fatalf("popup options need a touch target: %+v", first)
	}
	window.Move(first.X+first.W/2, first.Y+first.H/2)
	window.Scroll(first.X+first.W/2, first.Y+first.H/2, 0, 600)
	window.Advance(time.Second)
	last, ok := window.Find(label(options[len(options)-1]))
	if !ok || last.Y < 25 || last.Y+last.H > 365 {
		t.Fatalf("the last option must be within the usable area: %+v", last)
	}
	if err := window.Click(label(options[len(options)-1])); err != nil || selected != 10 {
		t.Fatalf("the last provider could not be reached by scrolling from %+v: selected %d, %v", first, selected, err)
	}
}

func TestCompactMCPServerKeepsActionsReachable(t *testing.T) {
	server := mcptools.Server{
		Name: strings.Repeat("long-server-name-", 8),
		URL:  "https://" + strings.Repeat("long-hostname.", 5) + "example.com/mcp",
	}
	opened := false
	window := ui.NewTester(func(c *ui.Context) {
		mcpServerRow(c, asseticon.Set{}, server, false, func() { opened = true }, func() {})
	}, 320, 240)
	window.SetRegion(i18n.Region{Locale: "en-US"})
	button, ok := window.Find(locales.McpServerActions())
	if !ok || button.W < 44 || button.H < 44 || button.X+button.W > 320 {
		t.Fatalf("service actions must remain visible and tappable: %+v", button)
	}
	if err := window.Click(locales.McpServerActions()); err != nil {
		t.Fatal(err)
	}
	if err := window.Click(locales.EditMcpServer()); err != nil || !opened {
		t.Fatalf("could not edit the service from its menu: %v", err)
	}
}

func TestCompactSettingsFooterKeepsSaveUnderLongError(t *testing.T) {
	message := strings.Repeat("The connection settings are invalid. ", 40)
	saved := false
	window := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Fill().Children(func() {
			ui.Box(c).Grow(1).MinHeight(0)
			Footer(c, message, true, func() { saved = true })
		})
	}, 320, 540)
	window.SetInsets(48, 8, 270, 8)
	button, ok := window.Find(locales.SaveSettings())
	if !ok || button.H < 44 || button.Y < 48 || button.Y+button.H > 270 {
		t.Fatalf("the save action must stay above the keyboard: %+v", button)
	}
	if err := window.Click(locales.SaveSettings()); err != nil || !saved {
		t.Fatalf("could not save with the status displayed: %v", err)
	}
}
