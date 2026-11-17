package localtools

import (
	"errors"
	"maps"

	"github.com/ggdream/microlight/assets/locales"
	search "github.com/ggdream/microlight/pkg/websearch"
)

// Migrate removes credentials for the retired provider and disables search if it was selected.
func (config *SearchConfig) Migrate() {
	const retired search.ID = "bocha"
	if config.Provider == retired {
		config.Enabled = false
		config.Provider = ""
	}
	if _, saved := config.Options[retired]; saved {
		config.Options = maps.Clone(config.Options)
		delete(config.Options, retired)
		if len(config.Options) == 0 {
			config.Options = nil
		}
	}
}

type SearchConfig struct {
	Enabled  bool                         `json:"enabled"`
	Provider search.ID                    `json:"provider,omitempty"`
	Options  map[search.ID]search.Options `json:"options,omitempty"`
}

func (config SearchConfig) Selected() search.ID {
	if config.Provider == "" {
		return search.DuckDuckGo
	}
	return config.Provider
}

func (config SearchConfig) Validate() error {
	for id, options := range config.Options {
		if err := validateSearchOptions(id, options, false); err != nil {
			return err
		}
	}
	id := config.Selected()
	return validateSearchOptions(id, config.Options[id], config.Enabled)
}

func validateSearchOptions(id search.ID, options search.Options, required bool) error {
	definition, ok := search.Lookup(id)
	if !ok {
		return errors.New(locales.InvalidSearchProvider())
	}
	switch err := definition.Validate(options, required); err {
	case search.ErrInvalidKey:
		return errors.New(locales.InvalidKey())
	case search.ErrMissingKey:
		return errors.New(locales.SearchKeyRequired())
	case search.ErrMissingEngineID:
		return errors.New(locales.SearchEngineIdRequired())
	case search.ErrInvalidURL:
		return errors.New(locales.InvalidSearchURL())
	default:
		return err
	}
}
