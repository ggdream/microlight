//go:build js && wasm

package chat

import (
	"bytes"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
)

// Browser SQL is asynchronous. MyGo's synchronous DOM/frame callback must return
// before worker messages can arrive. Serialize app actions outside those callbacks;
// nested actions stay on the same coroutine so their transactional order is kept.
// Browser Go executes on one JS thread. This queue is never used by native builds.
type browserActions struct {
	mu    sync.Mutex
	jobs  []func()
	wake  chan struct{}
	owner atomic.Uint64
}

// MyGo's Web backend uses this same runtime-stack technique to distinguish the
// main event-loop coroutine from callback coroutines. It is private to js/wasm.
func browserCoroutine() uint64 {
	var stack [64]byte
	n := runtime.Stack(stack[:], false)
	id, _ := strconv.ParseUint(string(bytes.Fields(stack[:n])[1]), 10, 64)
	return id
}

func (app *chatApp) deferBrowserAction(action func()) bool {
	if app.win == nil {
		return false
	} // startup is outside DOM callbacks
	queue := app.browserActions
	if queue == nil {
		queue = &browserActions{wake: make(chan struct{}, 1)}
		app.browserActions = queue
		go func() {
			queue.owner.Store(browserCoroutine())
			for {
				select {
				case <-app.appContext().Done():
					return
				case <-queue.wake:
				}
				for {
					queue.mu.Lock()
					if len(queue.jobs) == 0 {
						queue.mu.Unlock()
						break
					}
					job := queue.jobs[0]
					queue.jobs[0] = nil
					queue.jobs = queue.jobs[1:]
					queue.mu.Unlock()
					job()
					app.win.Invalidate()
				}
			}
		}()
	}
	if queue.owner.Load() == browserCoroutine() {
		return false
	}
	queue.mu.Lock()
	queue.jobs = append(queue.jobs, action)
	queue.mu.Unlock()
	select {
	case queue.wake <- struct{}{}:
	default:
	}
	return true
}
