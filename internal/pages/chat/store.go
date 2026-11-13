package chat

import (
	"github.com/ggdream/microlight/internal/session"
	"github.com/ggdream/microlight/internal/storage"
)

type localStore struct {
	session.Store
}

func openLocalStore(dir string) (*localStore, error) {
	db, err := storage.Open(dir)
	if err != nil {
		return nil, err
	}
	store := &localStore{Store: session.Store{DB: db}}
	if err := store.migrate(dir); err != nil {
		db.Close()
		return nil, err
	}
	return store, nil
}
