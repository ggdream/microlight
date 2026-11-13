package chat

import (
	"encoding/json"

	"github.com/ggdream/microlight/internal/storage"
	"github.com/ggdream/microlight/pkg/proxy"
)

func (store *localStore) loadSettings() (*modelSettings, error) {
	data, err := store.getConfig("settings")
	if err != nil || data == "" {
		return nil, err
	}
	var config modelSettings
	if err := json.Unmarshal([]byte(data), &config); err != nil {
		return nil, err
	}
	if err := config.validate(); err != nil {
		return nil, err
	}
	return &config, nil
}

const proxyConfigKey = "proxy"

func (store *localStore) loadProxy() (proxy.Config, error) {
	config := proxy.Config{Mode: proxy.System}
	raw, err := store.getConfig(proxyConfigKey)
	if err != nil {
		return config, err
	}
	if raw == "" {
		data, err := json.Marshal(config)
		if err != nil {
			return config, err
		}
		return config, storage.PutConfig(store.DB, proxyConfigKey, string(data))
	}
	err = json.Unmarshal([]byte(raw), &config)
	return config, err
}

func (store *localStore) saveSettings(config modelSettings) error {
	return store.saveConfiguration(config, nil)
}

func (store *localStore) saveConfiguration(config modelSettings, catalog *providerCatalog) error {
	data, err := json.Marshal(config)
	if err != nil {
		return err
	}
	config.Proxy.Mode = config.Proxy.EffectiveMode()
	network, err := json.Marshal(config.Proxy)
	if err != nil {
		return err
	}
	tx, err := store.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := storage.PutConfig(tx, "settings", string(data)); err != nil {
		return err
	}
	if err := storage.PutConfig(tx, proxyConfigKey, string(network)); err != nil {
		return err
	}
	if catalog != nil {
		providers, err := json.Marshal(catalog)
		if err != nil {
			return err
		}
		if err := storage.PutConfig(tx, providersConfigKey, string(providers)); err != nil {
			return err
		}
	}
	return tx.Commit()
}
