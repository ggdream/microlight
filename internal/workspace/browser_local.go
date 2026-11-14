package workspace

import (
	"context"
	"errors"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/chromedp/cdproto/target"
	"github.com/chromedp/chromedp"
)

type LocalBrowser struct {
	mu         sync.Mutex
	ctx        context.Context
	cancel     context.CancelFunc
	executable string
	allocator  func(context.Context) (context.Context, context.CancelFunc)
	closed     bool
}

// NewLocalBrowser prepares an isolated Chromium profile. It starts the
// browser only when an authorized browser tool first uses it.
func NewLocalBrowser() *LocalBrowser {
	candidates := []string{"chromium", "chromium-browser", "google-chrome", "chrome", "msedge"}
	switch runtime.GOOS {
	case "windows":
		for _, root := range []string{os.Getenv("ProgramFiles"), os.Getenv("ProgramFiles(x86)"), os.Getenv("LOCALAPPDATA")} {
			if root != "" {
				candidates = append(candidates, filepath.Join(root, "Google", "Chrome", "Application", "chrome.exe"), filepath.Join(root, "Microsoft", "Edge", "Application", "msedge.exe"))
			}
		}
	case "darwin":
		candidates = append(candidates, "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome", "/Applications/Chromium.app/Contents/MacOS/Chromium", "/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge")
	}
	for _, candidate := range candidates {
		if executable, err := exec.LookPath(candidate); err == nil {
			return &LocalBrowser{executable: executable}
		}
	}
	return nil
}

func (b *LocalBrowser) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.closed = true
	if b.cancel != nil {
		b.cancel()
		b.cancel = nil
		b.ctx = nil
	}
}

func (b *LocalBrowser) run(ctx context.Context, tabID string, action func(context.Context) error) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if b.closed {
		return errors.New("browser session closed")
	}
	ctx, limit := context.WithTimeout(ctx, 30*time.Second)
	defer limit()
	if b.ctx == nil {
		allocate := b.allocator
		if allocate == nil {
			allocate = func(ctx context.Context) (context.Context, context.CancelFunc) {
				return chromedp.NewExecAllocator(ctx, chromedp.ExecPath(b.executable), chromedp.Headless, chromedp.NoFirstRun,
					chromedp.Flag("no-sandbox", false), chromedp.Flag("disable-background-networking", true))
			}
		}
		alloc, stop := allocate(context.Background())
		browser, closeBrowser := chromedp.NewContext(alloc, chromedp.WithDetachOnCancel(), chromedp.WithErrorf(func(string, ...any) {}))
		cancel := func() { closeBrowser(); stop() }
		startup := context.AfterFunc(ctx, stop)
		var err error
		if b.allocator == nil {
			err = chromedp.Do(browser)
		} else {
			_, err = chromedp.Targets(browser)
		}
		if !startup() || err != nil || ctx.Err() != nil {
			cancel()
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return err
		}
		b.ctx, b.cancel = browser, cancel
	}
	current := b.ctx
	if tabID != "" {
		tab, detach := chromedp.NewContext(current, chromedp.WithTargetID(target.ID(tabID)), chromedp.WithDetachOnCancel())
		defer detach()
		current = tab
	}
	operation, cancel := context.WithCancel(current)
	defer cancel()
	stop := context.AfterFunc(ctx, cancel)
	defer stop()
	err := action(operation)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}

func (b *LocalBrowser) ListTabs(ctx context.Context) (tabs []TabInfo, err error) {
	err = b.run(ctx, "", func(ctx context.Context) error {
		entries, err := chromedp.Targets(ctx)
		for _, entry := range entries {
			if entry.Type == "page" {
				tabs = append(tabs, TabInfo{ID: string(entry.TargetID), URL: entry.URL, Title: entry.Title})
			}
		}
		return err
	})
	return
}

func (b *LocalBrowser) Navigate(ctx context.Context, tabID, rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil || u.Hostname() == "" || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return errors.New("expected an HTTP(S) URL without credentials")
	}
	if tabID == "" {
		return errors.New("tab ID is required")
	}
	return b.run(ctx, tabID, func(ctx context.Context) error { return chromedp.Do(ctx, chromedp.Navigate(u.String())) })
}

func (b *LocalBrowser) Snapshot(ctx context.Context, tabID string) (snapshot BrowserSnapshot, err error) {
	if tabID == "" {
		return snapshot, errors.New("tab ID is required")
	}
	err = b.run(ctx, tabID, func(ctx context.Context) error {
		var err error
		snapshot, err = chromedp.Run(ctx, chromedp.Evaluate[BrowserSnapshot](`(() => {
			const controls = [];
			for (const element of document.querySelectorAll("a[href],button,input,select,textarea,[role=button],[role=link]")) {
				if (!element.checkVisibility({visibilityProperty: true, opacityProperty: true}) ||
					element.matches(":disabled,[aria-disabled=true]")) continue;
				const path = [];
				for (let current = element; current; current = current.parentElement) {
					const id = current.id && "#" + CSS.escape(current.id);
					if (id && document.querySelectorAll(id).length === 1) {
						path.unshift(id);
						break;
					}
					let position = 1;
					for (let sibling = current.previousElementSibling; sibling; sibling = sibling.previousElementSibling) position++;
					path.unshift(CSS.escape(current.localName) + ":nth-child(" + position + ")");
				}
				const label = element.getAttribute("aria-label") || element.innerText ||
					element.getAttribute("title") || element.localName;
				controls.push({Selector: path.join(" > "), Label: label.trim().slice(0, 200)});
				if (controls.length === 100) break;
			}
			return {URL: location.href, Title: document.title,
				Text: (document.body?.innerText ?? "").slice(0, 20000), Controls: controls};
		})()`))
		return err
	})
	return
}

func (b *LocalBrowser) Click(ctx context.Context, tabID, selector string) error {
	if tabID == "" || selector == "" {
		return errors.New("tab ID and CSS selector are required")
	}
	return b.run(ctx, tabID, func(ctx context.Context) error { return chromedp.Do(ctx, chromedp.Click(chromedp.CSS(selector))) })
}
