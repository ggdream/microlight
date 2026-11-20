package settings

import (
	"fmt"
	"maps"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/ggdream/microlight/assets/locales"
	asseticon "github.com/ggdream/microlight/internal/icon"
	"github.com/ggdream/microlight/internal/localtools"
	"github.com/ggdream/microlight/internal/theme"
	"github.com/ggdream/microlight/pkg/asr"

	"github.com/ggdream/microlight/pkg/viewport"
)

type ASRForm struct {
	draft *localtools.ASRConfig
	err   string
}

func (f ASRForm) HasChanges(saved localtools.ASRConfig) bool {
	return f.draft != nil && (f.draft.Enabled != saved.Enabled || f.draft.Selected() != saved.Selected() || !maps.Equal(f.draft.Options, saved.Options))
}
func (f *ASRForm) View(c *ui.Context, icons asseticon.Set, saved localtools.ASRConfig, save func(localtools.ASRConfig) error) {
	if f.draft == nil {
		draft := saved
		draft.Provider = saved.Selected()
		draft.Options = maps.Clone(saved.Options)
		if draft.Options == nil {
			draft.Options = map[asr.ID]asr.Options{}
		}
		f.draft = &draft
	}
	draft := f.draft
	id := draft.Provider
	o := draft.Options[id]
	d, _ := asr.Lookup(id)
	changed := func() { draft.Options[id] = o; f.err = "" }
	reset := func() { f.err = "" }
	CardWithAction(c, locales.AsrTool(), func(c *ui.Context) { theme.Switch(c.Key("enabled"), &draft.Enabled, locales.EnableAsrTool(), reset) }, func() {
		ui.Text(c, locales.AsrToolHint()).FontSize(viewport.Font(c, 12)).TextColor(theme.Muted)
		settingsSelect(c.Key("provider"), icons, locales.AsrProvider(), &draft.Provider, asr.IDs(), func(id asr.ID) string { d, _ := asr.Lookup(id); return d.Name }, reset)
		fields := c.Key(string(id))
		keyLabel := "API Key"
		if id == asr.Tencent {
			keyLabel = "Secret ID"
		}
		if id == asr.Huawei {
			keyLabel = "IAM Token"
		}
		if id == asr.Google {
			keyLabel = "OAuth 2 Access Token"
		}
		Field(fields, keyLabel, "", &o.APIKey).Label(d.Name + " " + keyLabel).Password().OnChange(changed)
		if d.SecretKey {
			Field(fields, locales.AsrSecretKey(), "", &o.SecretKey).Label(d.Name + " " + locales.AsrSecretKey()).Password().OnChange(changed)
		}
		if d.AppID {
			Field(fields, "App ID", "", &o.AppID).OnChange(changed)
		}
		if d.ProjectID {
			Field(fields, "Project ID", "", &o.ProjectID).OnChange(changed)
		}
		if d.BaseURL {
			Field(fields, locales.AsrEndpoint(), d.Endpoint, &o.BaseURL).Keyboard(ui.KeyboardURL).Capitalize(ui.CapitalizeNone).Autocorrect(false).OnChange(changed)
		}
		if d.Model != "" {
			Field(fields, locales.ModelID(), d.Model, &o.Model).OnChange(changed)
		}
		if id != asr.DeepSeek && id != asr.Tencent && id != asr.Baidu && id != asr.Huawei && id != asr.Zhipu && id != asr.Volcengine {
			Field(fields, locales.AsrLanguage(), locales.AsrLanguageHint(), &o.Language).OnChange(changed)
		}
		if id == asr.Baidu || id == asr.Google {
			rate := strconv.Itoa(o.SampleRate)
			if o.SampleRate == 0 {
				rate = ""
			}
			Field(fields, locales.AsrSampleRate(), "16000", &rate).Keyboard(ui.KeyboardNumber).OnChange(func() {
				parsed, err := strconv.Atoi(rate)
				if err != nil && rate != "" {
					o.SampleRate = -1
					changed()
					f.err = locales.AsrInvalidSampleRate()
					return
				}
				o.SampleRate = parsed
				changed()
			})
		}
		limit := fmt.Sprintf("%s · %.1f MiB", strings.Join(d.Formats, ", "), float64(d.MaxBytes)/(1<<20))
		if d.MaxSeconds > 0 {
			limit += fmt.Sprintf(" · ≤ %ds", d.MaxSeconds)
		}
		ui.Text(c, limit).FontSize(viewport.Font(c, 11)).TextColor(theme.Muted)
		switch id {
		case asr.Volcengine:
			ui.Text(c, locales.AsrVolcengineHint()).FontSize(viewport.Font(c, 12)).TextColor(theme.Muted)
		case asr.Google, asr.Huawei:
			ui.Text(c, locales.AsrTokenHint()).FontSize(viewport.Font(c, 12)).TextColor(theme.Muted)
		}
		if f.err != "" {
			ui.Text(c, f.err).FontSize(viewport.Font(c, 12)).TextColor(theme.ErrorFG)
		}
		if f.HasChanges(saved) {
			ui.Row(c).FillWidth().Justify(ui.End).Children(func() {
				ActionButton(c.Key("save"), icons, locales.SaveSettings(), "check", false).OnClick(func() {
					config := *draft
					config.Options = maps.Clone(config.Options)
					for id, o := range config.Options {
						o.APIKey = strings.TrimSpace(o.APIKey)
						o.SecretKey = strings.TrimSpace(o.SecretKey)
						o.BaseURL = strings.TrimRight(strings.TrimSpace(o.BaseURL), "/")
						o.Model = strings.TrimSpace(o.Model)
						o.AppID = strings.TrimSpace(o.AppID)
						o.ProjectID = strings.TrimSpace(o.ProjectID)
						o.Language = strings.TrimSpace(o.Language)
						config.Options[id] = o
					}
					if err := save(config); err != nil {
						f.err = err.Error()
						return
					}
					*f = ASRForm{}
				})
			})
		}
	}).Gap(settingsLayout(c).Density(11, 6))

}
