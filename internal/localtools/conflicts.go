package localtools

import (
	"context"
	"errors"
	"os"
	"sync"
)

var activeWrites sync.Map
var errFileConflict = errors.New("file changed or is being written concurrently; read it again before making another change")

// FileVersions records the revisions read or written by a conversation.
type FileVersions struct{ seen sync.Map }

func (op localOperation) checkVersion(ctx context.Context) error {
	access, _ := ctx.Value(sessionAccessKey{}).(*Access)
	if access == nil || access.versions == nil {
		return nil
	}
	if previous, ok := access.versions.seen.Load(op.target); ok && previous != op.fileVersion {
		return errFileConflict
	}
	return nil
}

func (op localOperation) rememberVersion(ctx context.Context, info os.FileInfo) {
	access, _ := ctx.Value(sessionAccessKey{}).(*Access)
	if access != nil && access.versions != nil {
		access.versions.seen.Store(op.target, localFileVersion(info))
	}
}

func (op localOperation) reportConflict(ctx context.Context) {
	if report, ok := ctx.Value(reportToolResultKey{}).(func(Result)); ok && op.callID != "" {
		report(Result{CallID: op.callID, Target: op.target, Text: errFileConflict.Error(), Conflict: true})
	}
}
