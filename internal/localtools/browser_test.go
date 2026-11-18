package localtools

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/cloudwego/eino-ext/components/tool/browseruse"
	"github.com/ggdream/microlight/pkg/proxy"
)

func TestBrowserExternalUsesApprovedURLWithoutChromium(t *testing.T) {
	const target = "https://example.com/?a=1&b=2"
	const action = `{"action":"open_external","url":"https://example.com/?a=1&b=2"}`
	var opened []string
	browser := BrowserTool{openExternal: func(_ context.Context, target string) error {
		opened = append(opened, target)
		return nil
	}}
	for _, approved := range []string{"https://other.example/", target} {
		ctx := context.WithValue(t.Context(), approvedOperationKey{}, localOperation{name: BrowserUse, target: approved})
		result, err := browser.InvokableRun(ctx, action)
		if err != nil || strings.Contains(result, "desktop accepted") != (approved == target) {
			t.Fatalf("approval %q: %s, %v", approved, result, err)
		}
	}
	if len(opened) != 1 || opened[0] != target {
		t.Fatalf("desktop opened %v", opened)
	}
	ctx := context.WithValue(t.Context(), approvedOperationKey{}, localOperation{name: BrowserUse, target: target})
	if result, err := browser.InvokableRun(ctx, `{"action":"open_external","url":"file:///tmp/local"}`); err != nil ||
		!strings.Contains(result, "valid HTTP or HTTPS") || len(opened) != 1 {
		t.Fatalf("unexpected file URL result: %s, %v", result, err)
	}
}

type testBrowserEngine struct {
	fail browseruse.Action
	err  string
}

func (b testBrowserEngine) Execute(args *browseruse.Param) (*browseruse.ToolResult, error) {
	if args.Action == b.fail {
		return &browseruse.ToolResult{Error: b.err}, nil
	}
	return &browseruse.ToolResult{}, nil
}

func (testBrowserEngine) GetCurrentState() (*browseruse.BrowserState, error) {
	return &browseruse.BrowserState{URL: "https://example.com/", Tabs: []browseruse.TabInfo{{ID: 3}, {ID: 7}}}, nil
}

func TestBackgroundBrowserRecoversOnlyExplicitNavigation(t *testing.T) {
	for _, tt := range []struct {
		name, failure, firstResult string
		action                     browseruse.Action
		starts                     int
	}{
		{"devtools", "connection is closed", "isolated_background", browseruse.ActionGoToURL, 2},
		{"network", "net::ERR_CONNECTION_CLOSED", "website connection closed", browseruse.ActionGoToURL, 1},
		{"click", "connection is closed", "browser disconnected", browseruse.ActionClickElement, 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			session := NewBrowserSession(t.Context(), proxy.Config{}, BrowserBackground)
			t.Cleanup(session.Close)
			var starts int
			var first context.Context
			session.newEngine = func(ctx context.Context, config *browseruse.Config) (browserEngine, error) {
				if !config.Headless {
					return nil, errors.New("background browser is not headless")
				}
				starts++
				if starts == 1 {
					first = ctx
					return testBrowserEngine{tt.action, tt.failure}, nil
				}
				return testBrowserEngine{}, nil
			}
			const target = "https://example.com/"
			run := func(raw string) string {
				ctx := context.WithValue(WithBrowserSession(t.Context(), session), approvedOperationKey{},
					localOperation{name: BrowserUse, target: target})
				result, err := (BrowserTool{}).InvokableRun(ctx, raw)
				if err != nil {
					t.Fatal(err)
				}
				return result
			}
			result := run(`{"action":"go_to_url","url":"https://example.com/"}`)
			if tt.action == browseruse.ActionClickElement {
				if !strings.Contains(result, "isolated_background") {
					t.Fatal(result)
				}
				result = run(`{"action":"click_element","index":0}`)
			}
			if !strings.Contains(result, tt.firstResult) || starts != tt.starts {
				t.Fatalf("result=%q, starts=%d", result, starts)
			}
			if tt.name != "network" && (first.Err() == nil || session.ctx.Err() != nil) {
				t.Fatal("disconnect should cancel only the old browser process")
			}
			if tt.action == browseruse.ActionClickElement {
				if result = run(`{"action":"go_to_url","url":"https://example.com/"}`); !strings.Contains(result, "isolated_background") || starts != 2 {
					t.Fatalf("could not navigate again: %q (starts=%d)", result, starts)
				}
			}
		})
	}
}

func TestBrowserRunModeAndTabIndexes(t *testing.T) {
	const target = "https://example.com/"
	const navigate = `{"action":"go_to_url","url":"https://example.com/"}`
	const open = `{"action":"open_external","url":"https://example.com/"}`
	for _, tt := range []struct {
		name        string
		mode        BrowserMode
		wantBrowser string
		headless    bool
	}{
		{"default", "", "isolated_foreground", false},
		{"background", BrowserBackground, "isolated_background", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			session := NewBrowserSession(t.Context(), proxy.Config{}, tt.mode)
			t.Cleanup(session.Close)
			session.newEngine = func(_ context.Context, config *browseruse.Config) (browserEngine, error) {
				if config.Headless != tt.headless {
					t.Fatalf("headless = %t; want %t", config.Headless, tt.headless)
				}
				return testBrowserEngine{}, nil
			}
			opened := false
			browser := BrowserTool{Mode: tt.mode, openExternal: func(context.Context, string) error {
				opened = true
				return nil
			}}
			ctx := context.WithValue(WithBrowserSession(t.Context(), session), approvedOperationKey{},
				localOperation{name: BrowserUse, target: target})
			got, err := browser.InvokableRun(ctx, navigate)
			var status struct {
				Browser string
				Tabs    []browseruse.TabInfo
			}
			if err != nil || json.Unmarshal([]byte(got), &status) != nil || status.Browser != tt.wantBrowser ||
				len(status.Tabs) != 2 || status.Tabs[0].ID != 0 || status.Tabs[1].ID != 1 {
				t.Fatalf("unexpected browser state: %q, %v", got, err)
			}
			if _, err := browser.InvokableRun(ctx, open); err != nil || opened == tt.headless {
				t.Fatalf("external browser opened = %t in %s: %v", opened, tt.name, err)
			}
		})
	}
}

func TestBrowserExplicitModeOverridesDefaultForOneReply(t *testing.T) {
	const target = "https://example.com/"
	for _, tc := range []struct {
		configured, explicit BrowserMode
		headless             bool
	}{
		{BrowserForeground, BrowserBackground, true},
		{BrowserBackground, BrowserForeground, false},
	} {
		t.Run(string(tc.explicit), func(t *testing.T) {
			browser := BrowserTool{Mode: tc.configured}
			session := NewBrowserSession(t.Context(), proxy.Config{}, tc.configured)
			defer session.Close()
			var starts []bool
			session.newEngine = func(_ context.Context, config *browseruse.Config) (browserEngine, error) {
				starts = append(starts, config.Headless)
				return testBrowserEngine{}, nil
			}
			run := func(raw string, approvedMode BrowserMode) string {
				t.Helper()
				ctx := context.WithValue(WithBrowserSession(t.Context(), session), approvedOperationKey{},
					localOperation{name: BrowserUse, target: target, browserMode: approvedMode})
				got, err := browser.InvokableRun(ctx, raw)
				if err != nil {
					t.Fatal(err)
				}
				return got
			}
			navigate := `{"action":"go_to_url","url":"https://example.com/","mode":"` + string(tc.explicit) + `"}`
			if got := run(navigate, tc.configured); !strings.Contains(got, "changed after approval") || len(starts) != 0 {
				t.Fatalf("call with the wrong mode approval started a browser: %s, %v", got, starts)
			}
			want := "isolated_" + string(tc.explicit)
			if got := run(navigate, tc.explicit); !strings.Contains(got, want) {
				t.Fatal(got)
			}
			// Page interactions reuse the actual mode even if the configured setting differs.
			if got := run(`{"action":"get_state"}`, tc.explicit); !strings.Contains(got, want) || len(starts) != 1 || starts[0] != tc.headless {
				t.Fatalf("follow-up did not reuse the override: %s, headless modes %v", got, starts)
			}
			if got := run(`{"action":"click_element","index":0,"mode":"`+string(tc.configured)+`"}`, tc.configured); !strings.Contains(got, "requires a URL") {
				t.Fatalf("changing modes must not reuse element indexes: %s", got)
			}
			session.Close()
			session = NewBrowserSession(t.Context(), proxy.Config{}, tc.configured)
			defer session.Close()
			session.newEngine = func(_ context.Context, config *browseruse.Config) (browserEngine, error) {
				starts = append(starts, config.Headless)
				return testBrowserEngine{}, nil
			}
			if got := run(`{"action":"go_to_url","url":"https://example.com/"}`, tc.configured); !strings.Contains(got, "isolated_"+string(tc.configured)) || len(starts) != 2 || starts[1] == tc.headless {
				t.Fatalf("new reply did not use the configured default: %s, headless modes %v", got, starts)
			}
		})
	}
}
