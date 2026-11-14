package chat

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"time"

	"github.com/cloudwego/eino/adk/backgroundtask"
	backgroundtool "github.com/cloudwego/eino/adk/backgroundtask/tool"
	"github.com/cloudwego/eino/adk/filesystem"
	"github.com/egoist/mygo/ui"
	"github.com/ggdream/microlight/assets/locales"
	timeline "github.com/ggdream/microlight/internal/activity"
	"github.com/ggdream/microlight/internal/localtools"
	"github.com/ggdream/microlight/internal/workspace"
	"github.com/ggdream/microlight/pkg/elastic"
)

type savedRemoteCommand struct {
	localtools.RemoteCommand
	targetID string
	outcome  *backgroundtool.Outcome
}

// A fork inherits commands for turns before its fork point. The root branch
// used an empty ID before the conversation acquired its first explicit branch.
const commandLineage = `WITH RECURSIVE lineage(branch, inherited_before) AS (
	VALUES (?, ?)
	UNION ALL
	SELECT b.parent_id, min(lineage.inherited_before, b.fork_ordinal)
	FROM lineage JOIN conversation_branches b ON b.id = lineage.branch
	WHERE b.conversation_id = ? AND b.parent_id IS NOT NULL
)`

func (entry *turn) hasUnresolvedRemoteCommands() bool {
	return slices.ContainsFunc(entry.remoteCommands, func(record savedRemoteCommand) bool { return record.outcome == nil })
}

func (app *chatApp) remoteCommandControls(c *ui.Context, chat *conversation, entry *turn, ordinal int) {
	if entry.State == "pending" || !entry.hasUnresolvedRemoteCommands() {
		return
	}
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		for _, action := range []struct {
			label string
			stop  bool
		}{{locales.RemoteCommandReconnect(), false}, {locales.RemoteCommandStop(), true}} {
			disabled := app.active != nil || (entry.recoveringRemote && (!action.stop || entry.stopRemoteRecovery == nil))
			pointerButton(c.Key(action.label), disabled).Label(action.label).Background(side).Radius(7).Padding(6, 10).
				Children(func() { ui.Text(c, action.label).FontSize(12) }).
				OnClick(func() { app.recoverRemoteCommands(chat, entry, ordinal, action.stop) })
		}
		if entry.recoveringRemote {
			ui.Text(c, locales.RemoteCommandConnecting()).FontSize(12).TextColor(muted)
		}
	})
}

func (store *localStore) commandJournal(chatID, branch, targetID string, ordinal int) localtools.CommandJournal {
	return localtools.CommandJournal{
		Record: func(ctx context.Context, record localtools.RemoteCommand) error {
			result, err := store.ExecContext(ctx, `
				INSERT INTO remote_commands (id, conversation_id, turn_ordinal, branch_id, target_id, call_id, command)
				SELECT ?, c.id, t.ordinal, c.branch_id, ?, ?, ? FROM conversations c
				JOIN turns t ON t.conversation_id = c.id
				WHERE c.id = ? AND c.branch_id = ? AND t.ordinal = ? AND t.state = 'pending'
				ON CONFLICT(id) DO UPDATE SET id = excluded.id
				WHERE remote_commands.conversation_id = excluded.conversation_id
				AND remote_commands.branch_id = excluded.branch_id AND remote_commands.turn_ordinal = excluded.turn_ordinal
				AND remote_commands.target_id = excluded.target_id AND remote_commands.call_id = excluded.call_id
				AND remote_commands.command = excluded.command`,
				record.ID, targetID, record.CallID, record.Command, chatID, branch, ordinal)
			if err != nil {
				return err
			}
			if n, err := result.RowsAffected(); err != nil || n != 1 {
				return errors.Join(err, errors.New("the approved command no longer belongs to the active turn"))
			}
			return nil
		},
		Finish: store.finishRemoteCommand,
	}
}

func (store *localStore) finishRemoteCommand(ctx context.Context, id string, outcome *backgroundtool.Outcome) error {
	raw, err := json.Marshal(outcome)
	if err != nil {
		return err
	}
	_, err = store.ExecContext(ctx, "UPDATE remote_commands SET result = ? WHERE id = ? AND result IS NULL", raw, id)
	return err
}

func (store *localStore) loadRemoteCommands(chatID, branch string, ordinal int) ([]savedRemoteCommand, error) {
	return store.queryRemoteCommands(commandLineage+`
		SELECT id, call_id, command, target_id, result FROM remote_commands
		WHERE conversation_id = ? AND turn_ordinal = ? AND EXISTS (
			SELECT 1 FROM lineage WHERE turn_ordinal < inherited_before
			AND (branch_id = lineage.branch OR (branch_id = '' AND lineage.branch = ?))
		) ORDER BY rowid`, branch, ordinal+1, chatID, chatID, ordinal, chatID)
}

func (store *localStore) loadDetachedCommands(chat *conversation) ([]savedRemoteCommand, error) {
	return store.loadDetachedCommandsAt(chat.ID, chat.branchID, len(chat.Turns))
}

func (store *localStore) loadDetachedCommandsAt(chatID, branch string, turnCount int) ([]savedRemoteCommand, error) {
	return store.queryRemoteCommands(commandLineage+`
		SELECT id, call_id, command, target_id, result FROM remote_commands
		WHERE conversation_id = ? AND NOT EXISTS (
			SELECT 1 FROM lineage WHERE turn_ordinal < inherited_before
			AND (branch_id = lineage.branch OR (branch_id = '' AND lineage.branch = ?))
		)
		ORDER BY rowid`, branch, turnCount, chatID, chatID, chatID)
}

func (store *localStore) queryRemoteCommands(query string, args ...any) ([]savedRemoteCommand, error) {
	rows, err := store.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var records []savedRemoteCommand
	for rows.Next() {
		var record savedRemoteCommand
		var raw []byte
		if err := rows.Scan(&record.ID, &record.CallID, &record.Command, &record.targetID, &raw); err != nil {
			return nil, err
		}
		if len(raw) > 0 {
			if err := json.Unmarshal(raw, &record.outcome); err != nil {
				return nil, err
			}
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

func (entry *turn) applyRemoteCommands(records []savedRemoteCommand) {
	entry.remoteCommands = records
	for _, record := range records {
		index := -1
		for i, activity := range entry.Activity {
			if activity.CallID == record.CallID {
				index = i
				break
			}
		}
		if index < 0 {
			args, _ := json.Marshal(map[string]string{"command": record.Command})
			index = len(entry.Activity)
			entry.Activity = append(entry.Activity, timeline.Entry{CallID: record.CallID, Tool: localtools.Execute, Target: record.Command, Arguments: string(args)})
		}
		item := &entry.Activity[index]
		item.Pending = record.outcome == nil
		if record.outcome == nil {
			continue
		}
		result := localtools.CommandResult{}
		output := record.outcome.Error
		switch record.outcome.Status {
		case backgroundtask.StatusCompleted:
			var response filesystem.ExecuteResponse
			if err := json.Unmarshal(record.outcome.Data, &response); err == nil {
				result.ExitCode, result.TimedOut, result.Truncated = response.ExitCode, response.TimedOut, response.Truncated
				output = response.Output
			} else {
				result.Failed = true
				output = locales.HistoryReadFailed()
			}
		case backgroundtask.StatusCanceled:
			output = locales.RemoteCommandCanceled()
		default:
			result.Failed = true
		}
		item.Output, item.CommandResult = &output, &result
	}
}

func (entry *turn) applyDetachedCommands(records []savedRemoteCommand) {
	// Model-generated call IDs may repeat across branches.
	entry.Activity = nil
	display := slices.Clone(records)
	for i := range display {
		display[i].CallID = display[i].ID
	}
	entry.applyRemoteCommands(display)
	entry.remoteCommands = records
}

func (app *chatApp) recoverRemoteCommands(chat *conversation, entry *turn, ordinal int, stop bool) {
	if app.active != nil || app.store == nil {
		return
	}
	if entry.recoveringRemote {
		if stop && entry.stopRemoteRecovery != nil {
			entry.stopRemoteRecovery(localtools.ErrStopReply)
			entry.stopRemoteRecovery = nil
		}
		return
	}
	entry.recoveringRemote = true
	entry.Error = ""
	target, store := chat.target, app.store
	chatID, branch := chat.ID, chat.branchID
	turnCount := len(chat.Turns)
	secret := app.workspaceSecrets[target.Credential]
	records := append([]savedRemoteCommand(nil), entry.remoteCommands...)
	monitor, stopMonitor := context.WithCancelCause(context.Background())
	if !stop {
		entry.stopRemoteRecovery = stopMonitor
	}
	go func() {
		defer stopMonitor(nil)
		ctx, cancel := context.WithTimeout(monitor, 3*time.Minute)
		defer cancel()
		connect := func(ctx context.Context) (workspace.Runtime, error) { return workspace.Open(ctx, target, secret) }
		var failure error
		for _, record := range records {
			if record.outcome != nil {
				continue
			}
			if record.targetID != target.Identity() {
				failure = errors.New(locales.RemoteCommandTargetChanged())
				break
			}
			outcome, err := localtools.RecoverRemoteCommand(ctx, connect, record.RemoteCommand, stop)
			if !stop && errors.Is(context.Cause(monitor), localtools.ErrStopReply) {
				stopCtx, cancelStop := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancelStop()
				ctx, stop = stopCtx, true
				outcome, err = localtools.RecoverRemoteCommand(ctx, connect, record.RemoteCommand, true)
			}
			if errors.Is(err, backgroundtask.ErrNotFound) {
				err = errors.New(locales.RemoteCommandNotFound())
			}
			if err == nil {
				err = store.finishRemoteCommand(ctx, record.ID, outcome)
			}
			if err != nil {
				failure = err
				break
			}
		}
		var saved []savedRemoteCommand
		var err error
		if ordinal >= 0 {
			saved, err = store.loadRemoteCommands(chatID, branch, ordinal)
		} else {
			saved, err = store.loadDetachedCommandsAt(chatID, branch, turnCount)
		}
		app.win.Update(func() {
			entry.recoveringRemote = false
			entry.stopRemoteRecovery = nil
			if ordinal < 0 && chat.branchID != branch {
				app.refreshDetachedCommands(chat)
				return
			}
			if ordinal >= 0 && (chat.branchID != branch || ordinal >= len(chat.Turns) || chat.Turns[ordinal] != entry) {
				return
			}
			if err == nil {
				if ordinal < 0 {
					entry.applyDetachedCommands(saved)
				} else {
					entry.applyRemoteCommands(saved)
				}
			}
			if failure != nil {
				entry.Error = failure.Error()
			} else if err != nil {
				entry.Error = locales.HistoryReadFailed()
			}
			if ordinal >= 0 {
				app.save(chat)
			}
		})
	}()
}

func (app *chatApp) refreshDetachedCommands(chat *conversation) {
	if app.store == nil || chat.target.Kind == "" {
		return
	}
	records, err := app.store.loadDetachedCommands(chat)
	if err != nil {
		app.historyErr = locales.HistoryReadFailed()
		return
	}
	if chat.detachedCommands == nil && len(records) > 0 {
		chat.detachedCommands = &turn{State: "stopped"}
	}
	entry := chat.detachedCommands
	if entry != nil && !entry.recoveringRemote {
		entry.applyDetachedCommands(records)
	}
}

func (app *chatApp) detachedCommandsView(c *ui.Context, chat *conversation) {
	entry := chat.detachedCommands
	if entry == nil || len(entry.remoteCommands) == 0 {
		return
	}
	ui.Column(c).FillWidth().AlignItems(ui.Center).Padding(0, 26).Children(func() {
		ui.Column(c).FillWidth().MaxWidth(conversationWidth).Background(side).Padding(10).Radius(7).Gap(5).Children(func() {
			ui.Text(c, locales.RemoteCommandsPreviousBranch()).FontSize(12).FontWeight(620)
			scroll, offset := elastic.Scroll(c.Key("command-list"))
			scroll.FillWidth().MaxHeight(110).Children(func() {
				for i, item := range entry.Activity {
					label := item.Label(item.ShortTarget())
					offset.Apply(pointerButton(c.Key(i), false).Label(label).Padding(4, 6)).
						Children(func() { ui.Text(c, label).FontSize(12).TextColor(muted).MaxLines(1) }).
						OnClick(func() { app.openToolPreview(entry, i) })
				}
			})
			app.remoteCommandControls(c.Key("detached-controls"), chat, entry, -1)
			if entry.Error != "" {
				ui.Text(c, entry.Error).FontSize(12).TextColor(errorFG)
			}
		})
	})
}
