package session

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
)

type Store struct {
	*sql.DB
}

type Event = adk.SessionEvent[*schema.Message]

var (
	_          adk.SessionEventStore[*schema.Message] = (*Store)(nil)
	_          adk.CheckPointStore                    = (*Store)(nil)
	serializer                                        = &schema.HumanReadableSerializer{}
)

func (store *Store) AppendEvents(ctx context.Context, sessionID string, events []*Event) error {
	tx, err := store.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, event := range events {
		if event == nil || event.EventID == "" {
			return adk.ErrInvalidEventID
		}
		if err := adk.NormalizeSessionEventKind(event); err != nil {
			return err
		}
		data, err := serializer.Marshal(event)
		if err != nil {
			return err
		}
		result, err := tx.ExecContext(ctx, `
			INSERT INTO adk_session_events (session_id, event_id, kind, payload) VALUES (?, ?, ?, ?)
			ON CONFLICT(session_id, event_id) DO NOTHING
		`, sessionID, event.EventID, event.Kind, data)
		if err != nil {
			return err
		}
		inserted, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if inserted == 0 {
			return adk.ErrDuplicateEventID
		}
	}
	return tx.Commit()
}

func (store *Store) LoadEvents(ctx context.Context, sessionID string, req *adk.LoadSessionEventsRequest) (*adk.LoadSessionEventsResult[*schema.Message], error) {
	if req == nil {
		req = &adk.LoadSessionEventsRequest{}
	}
	query := "SELECT payload FROM adk_session_events WHERE session_id = ?"
	args := []any{sessionID}
	order, comparison := "ASC", ">"
	if req.Reverse {
		order, comparison = "DESC", "<"
	}
	if req.After != "" {
		var seq int64
		err := store.QueryRowContext(ctx, "SELECT seq FROM adk_session_events WHERE session_id = ? AND event_id = ?", sessionID, req.After).Scan(&seq)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, adk.ErrEventIDOutOfRange
		}
		if err != nil {
			return nil, err
		}
		query += " AND seq " + comparison + " ?"
		args = append(args, seq)
	}
	if len(req.Kinds) > 0 {
		query += " AND kind IN (?" + strings.Repeat(",?", len(req.Kinds)-1) + ")"
		for _, kind := range req.Kinds {
			args = append(args, kind)
		}
	}
	query += " ORDER BY seq " + order
	if req.Limit > 0 {
		query += " LIMIT ?"
		args = append(args, req.Limit)
	}
	rows, err := store.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := &adk.LoadSessionEventsResult[*schema.Message]{}
	for rows.Next() {
		var data []byte
		if err := rows.Scan(&data); err != nil {
			return nil, err
		}
		var event Event
		if err := serializer.Unmarshal(data, &event); err != nil {
			return nil, err
		}
		result.Events = append(result.Events, &event)
		result.Next = event.EventID
	}
	return result, rows.Err()
}

func (store *Store) Get(ctx context.Context, id string) ([]byte, bool, error) {
	var data []byte
	err := store.QueryRowContext(ctx, "SELECT payload FROM adk_checkpoints WHERE id = ?", id).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	return data, err == nil, err
}

func (store *Store) Set(ctx context.Context, id string, data []byte) error {
	sessionID, _ := adk.RunnerSessionID(ctx)
	_, err := store.ExecContext(ctx, `
		INSERT INTO adk_checkpoints (id, payload, session_id) VALUES (?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET payload = excluded.payload,
			session_id = COALESCE(NULLIF(excluded.session_id, ''), adk_checkpoints.session_id)
	`, id, data, sessionID)
	return err
}

func (store *Store) Delete(ctx context.Context, id string) error {
	_, err := store.ExecContext(ctx, "DELETE FROM adk_checkpoints WHERE id = ?", id)
	return err
}
