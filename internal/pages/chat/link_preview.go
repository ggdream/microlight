package chat

import (
	"context"
	"net/url"
	"sync"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/ggdream/microlight/assets/locales"
	"github.com/ggdream/microlight/internal/linkpreview"
	"github.com/ggdream/microlight/pkg/proxy"
)

const previewLeaveDelay = 220 * time.Millisecond

type previewRequest struct {
	url    string
	config proxy.Config
}

type previewResult struct {
	metadata linkpreview.Metadata
	until    time.Time
}

type linkPreviews struct {
	// The preview callback runs only when MyGo's delayed link tooltip opens.
	// Keep its destination until the pointer leaves both the link and the card.
	offered   string
	target    string
	dismissed string
	x, y      float32
	hideAt    time.Time

	mu       sync.Mutex
	results  map[previewRequest]previewResult
	inFlight map[previewRequest]bool
}

func (p *linkPreviews) offer(_ *ui.Context, dest string) func(ui.Element) {
	if _, ok := linkpreview.URL(dest); !ok {
		return nil
	}
	return func(tip ui.Element) {
		tip.Hidden(true)
		p.offered = dest
	}
}

func (app *chatApp) linkPreview(c *ui.Context, root ui.Element) {
	p := &app.links
	offer := p.offered
	p.offered = ""
	if offer == "" && p.target == "" {
		p.dismissed = ""
		return
	}
	x, y, pointerInside := root.PointerPosition()
	x, y = root.LocalToWindow(x, y)
	if !pointerInside || !c.WindowActive() || app.settingsOpen {
		p.target, p.dismissed, p.hideAt = "", "", time.Time{}
		return
	}
	if offer != p.dismissed {
		p.dismissed = ""
	}
	if offer != "" && offer != p.target && offer != p.dismissed {
		p.target, p.x, p.y, p.hideAt = offer, x, y, time.Time{}
	}
	if p.target == "" {
		return
	}

	target, _ := linkpreview.URL(p.target)
	now := c.Now()
	ui.Overlay(c, func() {
		anchor := ui.Box(c.Key("web-preview-anchor")).Absolute().Left(p.x).Top(p.y).
			Size(1, 1).IgnoreInput(true)
		panel := ui.Column(c.Key("web-preview-card")).Width(350).
			AttachTo(anchor, ui.AnchorTopLeft, ui.AnchorBottomLeft).AttachEdgeMargin(10).
			Margin(0, 0, 6, 0).Background(white).Radius(12).Border(1, border).
			Shadow(0, 6, 20, 0, ui.RGBA(0, 0, 0, .14))
		switch {
		case panel.OverlayShortcut(0, ui.KeyEscape) || panel.PressedOutside():
			p.dismiss(panel)
			return
		case offer == p.target || panel.Hovered() || panel.FocusWithin():
			p.hideAt = time.Time{}
		case p.hideAt.IsZero():
			p.hideAt = now.Add(previewLeaveDelay)
		case !p.hideAt.After(now):
			p.target = ""
			panel.Hidden(true)
			return
		}
		if !p.hideAt.IsZero() {
			c.After(p.hideAt.Sub(now))
		}
		metadata, finished := p.metadata(app.appContext(), c.Services(), previewRequest{target.String(), app.settings.Proxy})
		panel.Children(func() {
			p.cardHeader(c, target, panel)
			ui.Box(c).FillWidth().Height(1).Background(border)
			ui.Column(c).FillWidth().Padding(13, 15).Gap(5).Children(func() {
				if metadata.Title != "" {
					ui.Text(c, metadata.Title).FontSize(14).FontWeight(640).TextColor(ink).MaxLines(2)
				}
				switch {
				case metadata.Description != "":
					ui.Text(c, metadata.Description).FontSize(13).TextColor(muted).MaxLines(3)
				case !finished:
					ui.Text(c, locales.LinkPreviewLoading()).FontSize(13).TextColor(muted)
				case metadata.Title == "":
					ui.Text(c, p.target).FontSize(13).TextColor(muted).MaxLines(3)
				}
			})
		})
	})
}

func (p *linkPreviews) dismiss(panel ui.Element) {
	p.dismissed, p.target = p.target, ""
	panel.Hidden(true)
}

func (p *linkPreviews) cardHeader(c *ui.Context, target *url.URL, panel ui.Element) {
	ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(8).Padding(8, 10, 8, 15).Children(func() {
		glyph := "globe"
		if target.Hostname() == "github.com" {
			glyph = "github"
		}
		icon(c, glyph, muted, 15)
		ui.Text(c, target.Host).Grow(1).MinWidth(0).FontSize(13).TextColor(muted).SingleLine().Ellipsis("…")
		label := locales.LinkPreviewOpen()
		button := pointerButton(c, false).Label(label).AlignItems(ui.Center).Shrink(0).Gap(5).Padding(5, 6).Radius(6)
		if button.Hovered() {
			button.Background(hover)
		}
		button.Children(func() {
			ui.Text(c, label).FontSize(13).TextColor(ink)
			icon(c, "open", ink, 14)
		}).OnClick(func() {
			c.OpenURL(p.target)
			p.dismiss(panel)
		})
	})
}

func (p *linkPreviews) metadata(ctx context.Context, services ui.Services, request previewRequest) (linkpreview.Metadata, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	now := time.Now()
	if result, ok := p.results[request]; ok && result.until.After(now) {
		return result.metadata, true
	}
	if p.inFlight[request] || len(p.inFlight) >= 4 {
		return linkpreview.Metadata{}, false
	}
	if p.inFlight == nil {
		p.inFlight = make(map[previewRequest]bool)
	}
	p.inFlight[request] = true
	go func() {
		metadata, err := linkpreview.Fetch(ctx, request.url, request.config)
		ttl := 15 * time.Minute
		if err != nil {
			ttl = time.Minute
		}
		p.mu.Lock()
		if len(p.results) >= 128 || p.results == nil {
			p.results = make(map[previewRequest]previewResult)
		}
		delete(p.inFlight, request)
		if ctx.Err() != nil {
			p.mu.Unlock()
			return
		}
		p.results[request] = previewResult{metadata, time.Now().Add(ttl)}
		p.mu.Unlock()
		services.Invalidate()
	}()
	return linkpreview.Metadata{}, false
}
