package settings

import (
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/ggdream/microlight/assets/locales"
	asseticon "github.com/ggdream/microlight/internal/icon"
	"github.com/ggdream/microlight/internal/localtools"
	"github.com/ggdream/microlight/pkg/asr"
)

func TestASRSettingsPreserveProviderDrafts(t *testing.T) {
	var form ASRForm
	var saved localtools.ASRConfig
	window := ui.NewTester(func(c *ui.Context) {
		form.View(c, asseticon.Set{}, saved, func(next localtools.ASRConfig) error {
			if err := next.Validate(); err != nil {
				return err
			}
			saved = next
			return nil
		})
	}, 650, 1000)
	click := func(label string) {
		t.Helper()
		if err := window.Click(label); err != nil {
			t.Fatal(err)
		}
	}
	click(locales.EnableAsrTool())
	if saved.Enabled {
		t.Fatal("draft applied without saving")
	}
	click("DeepSeek API Key")
	window.Type("deepseek-test-key")
	click("DeepSeek")
	click("Xiaomi MiMo")
	click("Xiaomi MiMo API Key")
	window.Type("mimo-test-key")
	click(locales.SaveSettings())
	if !saved.Enabled || saved.Provider != asr.MiMo || saved.Options[asr.DeepSeek].APIKey != "deepseek-test-key" || saved.Options[asr.MiMo].APIKey != "mimo-test-key" {
		t.Fatalf("lost provider draft/options: saved=%+v draft=%+v error=%s", saved, form.draft, form.err)
	}
	if form.HasChanges(saved) {
		t.Fatal("saved form still marked dirty")
	}
}
