package chat

import (
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/ggdream/microlight/internal/pages/settings"
)

func TestProviderChipDragOrderCanBeDiscardedOrSaved(t *testing.T) {
	store, err := openLocalStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	app := &chatApp{store: store}
	app.loadSettings()
	first := app.providers.Providers[0]
	second, third := first, first
	first.ID, first.Name = "first", "First"
	second.ID, second.Name = "second", "Second"
	third.ID, third.Name = "third", "Third"
	catalog := providerCatalog{Providers: []modelProvider{first, second, third}, Selected: modelChoice{ProviderID: first.ID}}
	if err := app.saveProviderCatalog(catalog); err != nil {
		t.Fatal(err)
	}
	app.openSettings()
	app.settingsTab = settings.Model
	window := ui.NewTester(app.providerSettingsControl, 700, 180)
	drag := func(cancel, touch bool) {
		t.Helper()
		a, okA := window.AccessByRole(ui.RoleButton, "First")
		b, okB := window.AccessByRole(ui.RoleButton, "Third")
		if !okA || !okB {
			t.Fatal("provider chips missing")
		}
		x, y := a.Bounds.X+a.Bounds.W/2, a.Bounds.Y+a.Bounds.H/2
		targetX := b.Bounds.X + b.Bounds.W/2
		if touch {
			window.FingerDown(x, y)
		} else {
			window.Press(x, y)
		}
		for step := 1; step <= 4; step++ {
			nextX := x + (targetX-x)*float32(step)/4
			if touch {
				window.FingerMove(nextX, y)
			} else {
				window.Move(nextX, y)
			}
		}
		if cancel {
			window.Key(0, ui.KeyEscape)
		}
		if touch {
			window.FingerUp(targetX, y)
		} else {
			window.Release(targetX, y)
		}
		window.Frame()
	}
	drag(true, false)
	if app.hasModelChanges() {
		t.Fatal("canceling a drag must not change settings")
	}
	drag(false, false)
	want := []string{"second", "third", "first"}
	if !slices.Equal(app.form.providerOrder, want) || !app.hasModelChanges() {
		t.Fatalf("drag did not create the expected pending order: %v", app.form.providerOrder)
	}
	if !slices.Equal(providerIDs(app.providers.Providers), []string{"first", "second", "third"}) || app.form.providerID != "first" {
		t.Fatal("drag changed the saved catalog or selected provider")
	}
	app.discardSettingsChanges()
	window.Frame()
	if app.hasModelChanges() {
		t.Fatal("discard should restore saved order")
	}
	drag(false, true)
	app.saveSettings()
	if app.form.failed {
		t.Fatal(app.form.message)
	}
	reopened := &chatApp{store: store}
	reopened.loadSettings()
	if !slices.Equal(providerIDs(reopened.providers.Providers), want) || reopened.providers.Selected.ProviderID != "first" {
		t.Fatalf("saved order or selection did not survive reload: %+v", reopened.providers)
	}
	if app.hasModelChanges() {
		t.Fatal("successful save should clear pending order")
	}
}
