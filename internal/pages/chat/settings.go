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
	Protocol         apiProtocol       `json:"protocol,omitempty"`
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

type apiProtocol string

const (
	openAIResponses   apiProtocol = "openai_responses"
	openAIChat        apiProtocol = "openai_chat"
	anthropicMessages apiProtocol = "anthropic_messages"
)

func (s modelSettings) protocol() apiProtocol {
	if s.Protocol == "" {
		return openAIResponses
	}
	return s.Protocol
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
	if strings.HasSuffix(u.Path, "/chat/completions") || strings.HasSuffix(u.Path, "/responses") || strings.HasSuffix(u.Path, "/messages") {
		return errors.New(locales.BaseURLRoot())
	}
	switch s.protocol() {
	case openAIResponses, openAIChat, anthropicMessages:
		s.Protocol = s.protocol()
	default:
		return errors.New(locales.ClientInitFailed())
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
	proxyError        string
	modelForm
	providerDrafts      map[string]modelForm
	providerToDelete    string
	providerDeleteError string
	settingsStatus
	generalStatus settingsStatus
	test          *connectionTest
	leave         func()
}

type modelForm struct {
	protocol                     apiProtocol
	providerID, providerName     string
	baseURL, apiKey              string
	temperature, topP, maxTokens string
	contextThreshold             string
	models                       []*string
	defaultModel                 *string
	testModel                    string
	removeKey                    bool
}

func (form *modelForm) defaultModelID() string {
	if form.defaultModel == nil {
		return ""
	}
	return strings.TrimSpace(*form.defaultModel)
}

func (form modelForm) differsFrom(saved modelForm) bool {
	if form.protocol != saved.protocol || form.providerName != saved.providerName || form.baseURL != saved.baseURL ||
		form.apiKey != saved.apiKey || form.removeKey != saved.removeKey ||
		form.temperature != saved.temperature || form.topP != saved.topP ||
		form.maxTokens != saved.maxTokens || form.contextThreshold != saved.contextThreshold ||
		len(form.models) != len(saved.models) {
		return true
	}
	for i, row := range form.models {
		savedRow := saved.models[i]
		if *row != *savedRow || (row == form.defaultModel) != (savedRow == saved.defaultModel) {
			return true
		}
	}
	return false
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
	return modelSettings{Protocol: form.protocol, BaseURL: form.baseURL, Model: form.defaultModelID(), APIKey: key, Proxy: app.settings.Proxy}
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
	if app.providers.Providers != nil && len(app.providers.Providers) == 0 {
		app.backend.configErr = errors.New(locales.ProviderUnavailable())
	}
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
	form := modelForm{providerID: id, protocol: openAIResponses}
	s := app.providers.provider(id)
	if s == nil {
		form.defaultModel = new(string)
		form.models = []*string{form.defaultModel}
		return form
	}
	form.providerName, form.baseURL = s.Name, s.BaseURL
	form.protocol = s.protocol()
	form.temperature, form.topP = formatOptionalNumber(s.Temperature), formatOptionalNumber(s.TopP)
	if s.MaxTokens != nil {
		form.maxTokens = strconv.Itoa(*s.MaxTokens)
	}
	if s.ContextThreshold != 0 {
		form.contextThreshold = strconv.Itoa(s.ContextThreshold)
	}
	for _, model := range s.Models {
		row := new(model)
		form.models = append(form.models, row)
		if model == s.Model {
			form.defaultModel = row
		}
	}
	if form.defaultModel == nil {
		form.defaultModel = new(s.Model)
		form.models = append([]*string{form.defaultModel}, form.models...)
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

// deferSettingsLeave holds navigation until the user decides what to do with the draft.
func (app *chatApp) deferSettingsLeave(next func()) bool {
	if !app.settingsOpen {
		return false
	}
	if app.form.leave != nil {
		return true
	}
	switch app.settingsTab {
	case settings.Model:
		if !app.hasModelChanges() {
			return false
		}
	case settings.Tools:
		if !app.extensionsForm.mcp.HasChanges(app.settings.MCPServers) {
			return false
		}
	default:
		return false
	}
	app.form.leave = next
	return true
}

func (app *chatApp) hasModelChanges() bool {
	form := &app.form
	if form.modelForm.differsFrom(app.providerForm(form.providerID)) {
		return true
	}
	for id, draft := range form.providerDrafts {
		if id != form.providerID && draft.differsFrom(app.providerForm(id)) {
			return true
		}
	}
	return false
}

func (app *chatApp) discardSettingsChanges() {
	switch app.settingsTab {
	case settings.Model:
		app.clearConnectionTest()
		id := app.form.providerID
		if id == "" {
			id = app.providers.Selected.ProviderID
		}
		app.form.modelForm = app.providerForm(id)
		clear(app.form.providerDrafts)
		app.form.settingsStatus = settingsStatus{}
	case settings.Tools:
		app.extensionsForm.mcp = settings.MCPForm{}
	}
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
	app.form.proxyError = ""
	if config.Proxy == app.settings.Proxy {
		return
	}
	if err := app.applySettings(config); err != nil {
		app.form.proxyError = err.Error()
		return
	}
	app.clearConnectionTest()
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
	providers := app.providers.Providers
	if catalog != nil {
		providers = catalog.Providers
	}
	if providers != nil && len(providers) == 0 {
		backend.configErr = errors.New(locales.ProviderUnavailable())
	}
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

func (app *chatApp) testConnection(model string) {
	if test := app.form.test; test != nil && test.cancel != nil {
		return
	}
	config := app.connectionConfig()
	config.Model = model
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
