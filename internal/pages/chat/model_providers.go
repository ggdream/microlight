package chat

import (
	"encoding/json"
	"errors"
	"net/url"
	"slices"
	"strings"

	"github.com/ggdream/microlight/assets/locales"
	"github.com/ggdream/microlight/internal/storage"
	"github.com/ggdream/microlight/pkg/proxy"
)

const providersConfigKey = "model_providers"

type modelChoice struct {
	ProviderID string `json:"provider_id,omitempty"`
	Model      string `json:"model,omitempty"`
}

type modelProvider struct {
	ID     string   `json:"id"`
	Name   string   `json:"name"`
	Models []string `json:"models"`
	modelSettings
}

type providerCatalog struct {
	Providers []modelProvider `json:"providers"`
	Selected  modelChoice     `json:"selected"`
}

func (catalog providerCatalog) provider(id string) *modelProvider {
	for i := range catalog.Providers {
		if catalog.Providers[i].ID == id {
			return &catalog.Providers[i]
		}
	}
	return nil
}

func (p modelProvider) config(model string, shared modelSettings) modelSettings {
	config := p.modelSettings
	if model != "" {
		config.Model = model
	}
	config.Proxy, config.MCPServers = shared.Proxy, shared.MCPServers
	config.AccessMode, config.Workspace = shared.AccessMode, shared.Workspace
	return config
}

func (catalog providerCatalog) activeConfig(shared modelSettings) modelSettings {
	p := catalog.provider(catalog.Selected.ProviderID)
	if p == nil {
		return shared
	}
	return p.config(catalog.Selected.Model, shared)
}

func (app *chatApp) loadModelProviders() error {
	var catalog providerCatalog
	if app.store != nil {
		raw, err := app.store.getConfig(providersConfigKey)
		if err != nil {
			return err
		}
		if raw != "" {
			if err := json.Unmarshal([]byte(raw), &catalog); err != nil {
				return err
			}
			if len(catalog.Providers) == 0 && catalog.Selected == (modelChoice{}) {
				catalog.Providers = []modelProvider{}
				app.providers = catalog
				return nil
			}
			p := catalog.provider(catalog.Selected.ProviderID)
			if p == nil || (catalog.Selected.Model != "" && !slices.Contains(p.Models, catalog.Selected.Model)) {
				return errors.New(locales.SettingsReadFailed())
			}
			app.providers = catalog
			app.settings = catalog.activeConfig(app.settings)
			return nil
		}
	}

	config := app.settings
	config.Proxy, config.MCPServers = proxy.Config{}, nil
	config.AccessMode, config.Workspace = "", ""
	endpoint, _ := url.Parse(config.BaseURL)
	name := config.Model
	if endpoint != nil {
		switch endpoint.Hostname() {
		case "api.deepseek.com":
			name = "DeepSeek"
		case "api.openai.com":
			name = "OpenAI"
		default:
			if host := endpoint.Hostname(); host != "" {
				name = host
			}
		}
	}
	id := newID()
	app.providers = providerCatalog{
		Providers: []modelProvider{{ID: id, Name: name, Models: []string{config.Model}, modelSettings: config}},
		Selected:  modelChoice{ProviderID: id},
	}
	if app.store == nil {
		return nil
	}
	data, err := json.Marshal(app.providers)
	if err != nil {
		return err
	}
	return storage.PutConfig(app.store.DB, providersConfigKey, string(data))
}

func (app *chatApp) saveProviderCatalog(catalog providerCatalog) error {
	config := catalog.activeConfig(app.settings)
	if len(catalog.Providers) == 0 {
		config = modelSettings{
			BaseURL: defaultBaseURL, Model: defaultModel,
			Proxy: app.settings.Proxy, MCPServers: app.settings.MCPServers,
		}
	}
	if err := app.commitSettings(config, &catalog); err != nil {
		return err
	}
	app.providers = catalog
	app.providerError = ""
	return nil
}

func (app *chatApp) deleteProvider(id string) error {
	catalog := app.providers
	index := slices.IndexFunc(catalog.Providers, func(p modelProvider) bool { return p.ID == id })
	if index < 0 {
		return errors.New(locales.ProviderUnavailable())
	}
	catalog.Providers = slices.Delete(slices.Clone(catalog.Providers), index, index+1)
	if catalog.Selected.ProviderID == id {
		catalog.Selected = modelChoice{}
		if len(catalog.Providers) > 0 {
			catalog.Selected.ProviderID = catalog.Providers[0].ID
		}
	}
	if err := app.saveProviderCatalog(catalog); err != nil {
		return err
	}
	if app.form.providerID == id {
		app.editProvider(catalog.Selected.ProviderID)
	}
	delete(app.form.providerDrafts, id)
	return nil
}

func (app *chatApp) selectProvider(choice modelChoice) {
	if choice == app.providers.Selected {
		return
	}
	catalog := app.providers
	catalog.Selected = choice
	if err := app.saveProviderCatalog(catalog); err != nil {
		app.providerError = err.Error()
	}
}

func (app *chatApp) currentModelChoice() modelChoice {
	return modelChoice{ProviderID: app.providers.Selected.ProviderID, Model: app.settings.Model}
}

func (app *chatApp) backendFor(choice modelChoice) (*Chat, error) {
	if choice.ProviderID == "" || choice == app.currentModelChoice() {
		return app.backend, app.backend.configErr
	}
	p := app.providers.provider(choice.ProviderID)
	if p == nil {
		return nil, errors.New(locales.ProviderUnavailable())
	}
	config := p.config(choice.Model, app.settings)
	backend := newChat(config)
	backend.skillsHome = app.skillsHome
	return backend, backend.configErr
}

func (app *chatApp) providerFromForm(config modelSettings) (modelProvider, error) {
	form := &app.form
	name := strings.TrimSpace(form.providerName)
	if name == "" {
		return modelProvider{}, errors.New(locales.ProviderNameRequired())
	}
	for _, p := range app.providers.Providers {
		if p.ID != form.providerID && strings.EqualFold(p.Name, name) {
			return modelProvider{}, errors.New(locales.ProviderNameDuplicate())
		}
	}
	models := make([]string, 0, len(form.models))
	for _, raw := range form.models {
		model := strings.TrimSpace(*raw)
		if model == "" {
			continue
		}
		if slices.Contains(models, model) {
			return modelProvider{}, errors.New(locales.ModelIDDuplicate())
		}
		models = append(models, model)
	}
	id := form.providerID
	if id == "" {
		id = newID()
	}
	config.Proxy, config.MCPServers = proxy.Config{}, nil
	config.AccessMode, config.Workspace = "", ""
	return modelProvider{ID: id, Name: name, Models: models, modelSettings: config}, nil
}
