package chat

import (
	"context"
	"errors"
	"math"
	"net/url"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/ggdream/microlight/assets/locales"
	"github.com/ggdream/microlight/internal/localtools"
	"github.com/ggdream/microlight/internal/mcptools"
	"github.com/ggdream/microlight/internal/pages/settings"
	"github.com/ggdream/microlight/pkg/proxy"
)

type modelSettings struct {
	Proxy            proxy.Config      `json:"-"`
	MCPServers       []mcptools.Server `json:"mcp_servers,omitempty"`
	BaseURL          string            `json:"base_url"`
	Model            string            `json:"model"`
	APIKey           string            `json:"api_key,omitempty"`
	Temperature      *float32          `json:"temperature,omitempty"`
	TopP             *float32          `json:"top_p,omitempty"`
	ContextThreshold int               `json:"context_threshold,omitempty"`
	MaxTokens        *int              `json:"max_tokens,omitempty"`
	AccessMode       localtools.Mode   `json:"access_mode,omitempty"`
	Workspace        string            `json:"workspace,omitempty"`
}

func cleanBaseURL(raw string) string {
	return strings.TrimRight(strings.TrimSpace(raw), "/")
}

func (s *modelSettings) validate() error {
	s.BaseURL = cleanBaseURL(s.BaseURL)
	s.Model = strings.TrimSpace(s.Model)
	s.APIKey = strings.TrimSpace(s.APIKey)
	s.AccessMode = s.AccessMode.Normalized()
	s.Workspace = strings.TrimSpace(s.Workspace)
	seen := make(map[string]bool)
	for _, server := range s.MCPServers {
		if !mcptools.ServerName.MatchString(server.Name) || seen[server.Name] {
			return errors.New(locales.InvalidMCPServer())
		}
		if err := server.Validate(); err != nil {
			return err
		}
		seen[server.Name] = true
	}
	if s.Workspace != "" && !filepath.IsAbs(s.Workspace) {
		return errors.New(locales.AbsoluteWorkspace())
	}
	u, err := url.Parse(s.BaseURL)
	if err != nil || u.Hostname() == "" || (u.Scheme != "https" && u.Scheme != "http") ||
		u.User != nil || u.RawQuery != "" || u.ForceQuery || strings.Contains(s.BaseURL, "#") {
		return errors.New(locales.InvalidBaseURL())
	}
	if strings.HasSuffix(u.Path, "/chat/completions") {
		return errors.New(locales.BaseURLRoot())
	}
	if s.Model == "" {
		return errors.New(locales.RequiredModel())
	}
	if strings.ContainsAny(s.APIKey, "\r\n") {
		return errors.New(locales.InvalidKey())
	}
	if !inRange(s.Temperature, 2) {
		return errors.New(locales.InvalidTemperature())
	}
	if !inRange(s.TopP, 1) {
		return errors.New(locales.InvalidTopP())
	}
	if s.MaxTokens != nil && *s.MaxTokens <= 0 {
		return errors.New(locales.InvalidMaxTokens())
	}
	if s.ContextThreshold != 0 && s.ContextThreshold < 2048 {
		return errors.New(locales.InvalidThreshold())
	}
	return nil
}

func inRange(value *float32, max float32) bool {
	return value == nil || (*value >= 0 && *value <= max)
}

type settingsForm struct {
	proxy             proxy.Config
	proxyLaunchFailed bool
	modelForm
	providerDrafts map[string]modelForm
	settingsStatus
	generalStatus settingsStatus
	test          *connectionTest
}

type modelForm struct {
	providerID, providerName     string
	baseURL, model, apiKey       string
	temperature, topP, maxTokens string
	contextThreshold             string
	extraModels                  []*string
	removeKey                    bool
}

type settingsStatus struct {
	message string
	failed  bool
}

type connectionTest struct {
	cancel  context.CancelFunc
	message string
	failed  bool
}

func (app *chatApp) savedKeyForForm() string {
	p := app.providers.provider(app.form.providerID)
	if p != nil && !app.form.removeKey && p.BaseURL == cleanBaseURL(app.form.baseURL) {
		return p.APIKey
	}
	return ""
}

func (app *chatApp) connectionConfig() modelSettings {
	form := &app.form
	key := strings.TrimSpace(form.apiKey)
	if key == "" {
		key = app.savedKeyForForm()
	}
	return modelSettings{BaseURL: form.baseURL, Model: form.model, APIKey: key, Proxy: app.settings.Proxy}
}

func (form *settingsForm) proxyConfig() proxy.Config {
	network := form.proxy
	network.Mode = network.EffectiveMode()
	if network.Mode != proxy.Custom {
		network.URL = ""
	} else {
		network.URL = strings.TrimSpace(network.URL)
	}
	return network
}

func (app *chatApp) loadSettings() {
	app.settings = modelSettings{BaseURL: defaultBaseURL, Model: defaultModel}
	if app.store != nil {
		saved, err := app.store.loadSettings()
		switch {
		case err != nil:
			app.settingsLoadErr = locales.SettingsReadFailed()
		case saved != nil:
			app.settings = *saved
		}
		network, err := app.store.loadProxy()
		if err != nil {
			app.settingsLoadErr = locales.ProxyReadFailed()
		} else {
			app.settings.Proxy = network
		}
	}
	if err := app.loadModelProviders(); err != nil {
		app.settingsLoadErr = locales.SettingsReadFailed()
	}
	app.backend = newChat(app.settings)
	app.backend.skillsHome = app.skillsHome
}

func (app *chatApp) openSettings() {
	if app.settingsOpen {
		return
	}
	app.resetComposerSkills()
	status := settingsStatus{message: app.settingsLoadErr, failed: app.settingsLoadErr != ""}
	app.form = settingsForm{
		proxy: app.settings.Proxy, settingsStatus: status, generalStatus: status,
		providerDrafts: make(map[string]modelForm),
	}
	app.form.modelForm = app.providerForm(app.providers.Selected.ProviderID)
	app.settingsTab = settings.General
	app.extensionsForm = extensionsForm{}
	app.settingsOpen = true
}

func (app *chatApp) providerForm(id string) modelForm {
	form := modelForm{providerID: id}
	s := app.providers.provider(id)
	if s == nil {
		return form
	}
	form.providerName, form.baseURL, form.model = s.Name, s.BaseURL, s.Model
	form.temperature, form.topP = formatOptionalNumber(s.Temperature), formatOptionalNumber(s.TopP)
	if s.MaxTokens != nil {
		form.maxTokens = strconv.Itoa(*s.MaxTokens)
	}
	if s.ContextThreshold != 0 {
		form.contextThreshold = strconv.Itoa(s.ContextThreshold)
	}
	for _, model := range s.Models {
		if model != s.Model {
			form.extraModels = append(form.extraModels, new(model))
		}
	}
	return form
}

func (app *chatApp) editProvider(id string) {
	form := &app.form
	if id == form.providerID {
		return
	}
	form.providerDrafts[form.providerID] = form.modelForm
	draft, ok := form.providerDrafts[id]
	if !ok {
		draft = app.providerForm(id)
	}
	app.clearConnectionTest()
	form.modelForm, form.settingsStatus = draft, settingsStatus{}
}

func (app *chatApp) closeSettings() {
	app.clearConnectionTest()
	app.settingsOpen = false
	app.form = settingsForm{}
}

func formatOptionalNumber(value *float32) string {
	if value == nil {
		return ""
	}
	return strconv.FormatFloat(float64(*value), 'f', -1, 32)
}

func optionalNumber(raw string) *float32 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	value, err := strconv.ParseFloat(raw, 32)
	if err != nil {
		value = math.NaN()
	}
	parsed := float32(value)
	return &parsed
}

func (app *chatApp) saveSettings() {
	form := &app.form
	config := app.connectionConfig()
	config.MCPServers = app.settings.MCPServers
	config.Temperature = optionalNumber(form.temperature)
	config.TopP = optionalNumber(form.topP)
	if value := strings.TrimSpace(form.maxTokens); value != "" {
		n, err := strconv.Atoi(value)
		if err != nil {
			form.message, form.failed = locales.InvalidMaxTokens(), true
			return
		}
		config.MaxTokens = &n
	}
	if value := strings.TrimSpace(form.contextThreshold); value != "" {
		n, err := strconv.Atoi(value)
		if err != nil || n < 2048 {
			form.message, form.failed = locales.InvalidThreshold(), true
			return
		}
		config.ContextThreshold = n
	}
	if err := config.validate(); err != nil {
		form.settingsStatus = settingsStatus{message: err.Error(), failed: true}
		return
	}
	p, err := app.providerFromForm(config)
	if err != nil {
		form.settingsStatus = settingsStatus{message: err.Error(), failed: true}
		return
	}
	catalog := app.providers
	catalog.Providers = slices.Clone(catalog.Providers)
	if index := slices.IndexFunc(catalog.Providers, func(old modelProvider) bool { return old.ID == p.ID }); index >= 0 {
		catalog.Providers[index] = p
	} else {
		catalog.Providers = append(catalog.Providers, p)
	}
	if catalog.Selected.ProviderID == "" {
		catalog.Selected = modelChoice{ProviderID: p.ID}
	} else if catalog.Selected.ProviderID == p.ID && !slices.Contains(p.Models, catalog.Selected.Model) {
		catalog.Selected.Model = ""
	}
	if err := app.saveProviderCatalog(catalog); err != nil {
		form.settingsStatus = settingsStatus{message: err.Error(), failed: true}
		return
	}
	delete(form.providerDrafts, form.providerID)
	form.modelForm = app.providerForm(p.ID)
	form.settingsStatus = settingsStatus{message: locales.SettingsSaved()}
}

func (app *chatApp) saveProxySettings() {
	config := app.settings
	config.Proxy = app.form.proxyConfig()
	if err := app.applySettings(config); err != nil {
		app.form.generalStatus = settingsStatus{message: err.Error(), failed: true}
		return
	}
	app.form.proxy = config.Proxy
	app.clearConnectionTest()
	app.form.generalStatus = settingsStatus{message: locales.SettingsSaved()}
}

func (app *chatApp) applySettings(config modelSettings) error {
	return app.commitSettings(config, nil)
}

func (app *chatApp) commitSettings(config modelSettings, catalog *providerCatalog) error {
	if err := config.validate(); err != nil {
		return err
	}
	backend := newChat(config)
	if backend.configErr != nil {
		return backend.configErr
	}
	if app.store == nil || app.store.saveConfiguration(config, catalog) != nil {
		return errors.New(locales.SettingsSaveFailed())
	}
	backend.skillsHome = app.skillsHome
	if app.backend != nil && app.backend.network != nil {
		app.backend.network.CloseIdleConnections()
	}
	app.settings, app.backend, app.settingsLoadErr = config, backend, ""
	return nil
}

func (app *chatApp) clearConnectionTest() {
	if test := app.form.test; test != nil && test.cancel != nil {
		test.cancel()
	}
	app.form.test = nil
}

func (app *chatApp) removeSavedKey() {
	app.form.apiKey = ""
	app.form.removeKey = true
	app.clearConnectionTest()
}

func (app *chatApp) testConnection() {
	if test := app.form.test; test != nil && test.cancel != nil {
		return
	}
	config := app.connectionConfig()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	test := &connectionTest{cancel: cancel, message: locales.TestingConnection()}
	app.form.test = test
	go func() {
		defer cancel()
		err := checkConnection(ctx, config)
		app.win.Update(func() {
			if app.form.test != test {
				return
			}
			test.cancel = nil
			test.message = locales.ConnectionOK()
			if err != nil {
				test.message = locales.ConnectionFailedDetail(err.Error())
				test.failed = true
			}
		})
	}()
}

func checkConnection(ctx context.Context, config modelSettings) error {
	if err := config.validate(); err != nil {
		return err
	}
	backend := newChat(config)
	if backend.network != nil {
		defer backend.network.CloseIdleConnections()
	}
	return backend.stream(ctx, []message{{Role: "user", Content: "Reply only with OK."}}, func(replyDelta) error {
		return nil
	})
}
