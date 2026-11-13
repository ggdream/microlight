package chat

import (
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"

	"github.com/ggdream/microlight/internal/storage"
)

func (store *localStore) migrate(dir string) error {
	tx, err := store.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var version int
	if err := tx.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	switch version {
	case 12:
		return nil
	case 0:
		if err := createBaseSchema(tx); err != nil {
			return err
		}
		fallthrough
	case 1:
		if _, err := tx.Exec(`
			CREATE TABLE turn_images (
				conversation_id TEXT NOT NULL, turn_ordinal INTEGER NOT NULL, ordinal INTEGER NOT NULL,
				name TEXT NOT NULL, mime TEXT NOT NULL, data BLOB NOT NULL,
				PRIMARY KEY (conversation_id, turn_ordinal, ordinal),
				FOREIGN KEY (conversation_id, turn_ordinal) REFERENCES turns(conversation_id, ordinal)
			);
		`); err != nil {
			return err
		}
		fallthrough
	case 2:
		if _, err := tx.Exec(`
			ALTER TABLE turns ADD COLUMN reasoning TEXT NOT NULL DEFAULT '';
			ALTER TABLE turns ADD COLUMN started_at INTEGER;
			ALTER TABLE turns ADD COLUMN finished_at INTEGER;
		`); err != nil {
			return err
		}
		fallthrough
	case 3:
		if _, err := tx.Exec(`
			ALTER TABLE turns ADD COLUMN clarifications BLOB;
		`); err != nil {
			return err
		}
		fallthrough
	case 4:
		if _, err := tx.Exec(`
			ALTER TABLE conversations ADD COLUMN context_summary TEXT NOT NULL DEFAULT '';
			ALTER TABLE conversations ADD COLUMN context_through INTEGER NOT NULL DEFAULT 0;

		`); err != nil {
			return err
		}
		fallthrough
	case 5:
		if _, err := tx.Exec(`
			ALTER TABLE conversations DROP COLUMN context_summary;
			ALTER TABLE conversations DROP COLUMN context_through;
			ALTER TABLE conversations ADD COLUMN session_id TEXT NOT NULL DEFAULT '';
			CREATE TABLE adk_session_events (
				seq INTEGER PRIMARY KEY, session_id TEXT NOT NULL,
				event_id TEXT NOT NULL, kind TEXT NOT NULL, payload BLOB NOT NULL,
				UNIQUE (session_id, event_id)
			);
			CREATE INDEX adk_session_order ON adk_session_events(session_id, seq);
			CREATE TABLE adk_checkpoints (id TEXT PRIMARY KEY, payload BLOB NOT NULL);
		`); err != nil {
			return err
		}
		fallthrough
	case 6:
		if _, err := tx.Exec(`
			CREATE TABLE config (key TEXT PRIMARY KEY, value TEXT NOT NULL);
			INSERT INTO config (key, value)
				SELECT key, CAST(value AS TEXT) FROM app_data WHERE key = 'settings';
			DELETE FROM app_data WHERE key = 'settings';
		`); err != nil {
			return err
		}
		fallthrough
	case 7:
		if _, err := tx.Exec(`
			ALTER TABLE conversations ADD COLUMN access_mode TEXT NOT NULL DEFAULT 'ask';
			ALTER TABLE conversations ADD COLUMN workspace TEXT NOT NULL DEFAULT '';
		`); err != nil {
			return err
		}
		fallthrough
	case 8:
		if _, err := tx.Exec(`
			ALTER TABLE turns ADD COLUMN activity BLOB;
			PRAGMA user_version = 9;
		`); err != nil {
			return err
		}
		fallthrough
	case 9:
		if _, err := tx.Exec(`
			ALTER TABLE conversations ADD COLUMN pinned INTEGER NOT NULL DEFAULT 0;
			ALTER TABLE conversations ADD COLUMN archived INTEGER NOT NULL DEFAULT 0;
			PRAGMA user_version = 10;
		`); err != nil {
			return err
		}
		fallthrough
	case 10:
		if _, err := tx.Exec(`
			CREATE TABLE adk_session_owners (
				session_id TEXT PRIMARY KEY,
				conversation_id TEXT NOT NULL REFERENCES conversations(id)
			);
			CREATE INDEX adk_sessions_by_conversation ON adk_session_owners(conversation_id);
			INSERT INTO adk_session_owners (session_id, conversation_id)
				SELECT session_id, id FROM conversations WHERE session_id <> '';
			ALTER TABLE adk_checkpoints ADD COLUMN session_id TEXT NOT NULL DEFAULT '';
			CREATE INDEX adk_checkpoints_by_session ON adk_checkpoints(session_id);
			PRAGMA user_version = 11;
		`); err != nil {
			return err
		}
		fallthrough
	case 11:
		if _, err := tx.Exec(`
			ALTER TABLE conversations ADD COLUMN branch_id TEXT NOT NULL DEFAULT '';
			CREATE TABLE conversation_branches (
				id TEXT PRIMARY KEY,
				conversation_id TEXT NOT NULL REFERENCES conversations(id),
				parent_id TEXT REFERENCES conversation_branches(id),
				fork_ordinal INTEGER,
				session_id TEXT NOT NULL DEFAULT '',
				turns BLOB
			);
			CREATE INDEX conversation_branches_by_chat ON conversation_branches(conversation_id, parent_id);
			PRAGMA user_version = 12;
		`); err != nil {
			return err
		}
	default:
		return errors.New("unsupported database version")
	}
	if version == 0 {
		if err := importLegacy(tx, dir); err != nil {
			return err
		}
	}
	if version >= 8 {
		return tx.Commit()
	}
	// Existing conversations previously shared the global tool policy.
	var raw string
	if err := tx.QueryRow("SELECT value FROM config WHERE key = 'settings'").Scan(&raw); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	var previous modelSettings
	if raw != "" && json.Unmarshal([]byte(raw), &previous) == nil {
		if !filepath.IsAbs(previous.Workspace) {
			previous.Workspace = ""
		}
		if _, err := tx.Exec("UPDATE conversations SET access_mode = ?, workspace = ?", previous.AccessMode.Normalized(), previous.Workspace); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func createBaseSchema(tx *sql.Tx) error {
	_, err := tx.Exec(`
		CREATE TABLE app_data (key TEXT PRIMARY KEY, value BLOB NOT NULL);
		CREATE TABLE conversations (id TEXT PRIMARY KEY, title TEXT NOT NULL, position INTEGER NOT NULL);
		CREATE TABLE turns (
			conversation_id TEXT NOT NULL REFERENCES conversations(id),
			ordinal INTEGER NOT NULL, role TEXT NOT NULL, content TEXT NOT NULL,
			state TEXT NOT NULL, error TEXT NOT NULL,
			PRIMARY KEY (conversation_id, ordinal)
		);
	`)
	return err
}

func importLegacy(tx *sql.Tx, dir string) error {
	var oldHistory history
	if found, err := storage.ReadLegacy(dir, "conversations.json", &oldHistory); err != nil {
		return err
	} else if found {
		for index, chat := range oldHistory.Chats {
			if err := putConversation(tx, chat, len(oldHistory.Chats)-index); err != nil {
				return err
			}
		}
		if err := storage.PutData(tx, "selected", []byte(oldHistory.Selected)); err != nil {
			return err
		}
	}
	var config modelSettings
	if found, err := storage.ReadLegacy(dir, "settings.json", &config); err != nil {
		return err
	} else if found {
		if err := config.validate(); err != nil {
			return err
		}
		data, err := json.Marshal(config)
		if err != nil {
			return err
		}
		if err := storage.PutConfig(tx, "settings", string(data)); err != nil {
			return err
		}
	}
	return nil
}
