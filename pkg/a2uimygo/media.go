package a2uimygo

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/ggdream/microlight/pkg/a2ui"

	"github.com/egoist/mygo/audio"
	_ "github.com/egoist/mygo/audio/flac"
	_ "github.com/egoist/mygo/audio/mp3"
	_ "github.com/egoist/mygo/audio/mp4"
	_ "github.com/egoist/mygo/audio/vorbis"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/video"
	_ "golang.org/x/image/webp"
)

type MediaOptions struct {
	// Client enables remote media using the host's network policy. With nil,
	// only image data URLs and host-supplied media are loaded.
	Client *http.Client
	// RequireActivation delays remote requests until a click. Data images load locally.
	RequireActivation           bool
	LoadLabel, UnavailableLabel string
	Icon                        func(string) *ui.SVG
	// DecodeVideo optionally overrides MyGo's default video decoder.
	DecodeVideo func(context.Context, []byte) (ui.VideoDecoder, error)
}

type mediaSource struct {
	mu        sync.Mutex
	cancel    context.CancelFunc
	closed    bool
	ready     bool
	image     *ui.Bitmap
	audio     *audio.Player
	decoder   ui.VideoDecoder
	videoData []byte
	video     *ui.VideoPlayer
	err       error
}

func (s *mediaSource) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	s.cancel()
	if s.audio != nil {
		s.audio.Close()
	}
	if s.video != nil {
		s.video.Close()
	} else if closer, ok := s.decoder.(io.Closer); ok {
		_ = closer.Close()
	}
	return nil
}

func (r renderer) media(c *ui.Context, node a2ui.Component, scope string) {
	source := r.surface.String(node.Props["url"], scope)
	description := r.surface.String(node.Props["description"], scope)
	container := r.decorate(c, node, scope, ui.Column(c).Gap(5).MaxWidth(360), description)
	var media mediaFrame
	activation := r.Media.RequireActivation && (node.Type != "Image" || !strings.HasPrefix(source, "data:image/"))
	poster := r.surface.String(node.Props["posterUrl"], scope)
	loaded := ui.Local(container, struct{ Source, Poster string }{source, poster}, func() bool { return false })
	if !activation || *loaded {
		media = r.loadMedia(c, container, node.Type+":"+source, node.Type, source)
	}
	container.Children(func() {
		if description != "" && (node.Type == "AudioPlayer" || (activation && !*loaded)) {
			ui.Text(c, description).FontSize(12).TextColor(r.Style.Text)
		}
		if activation && !*loaded {
			label := r.Media.LoadLabel
			if label == "" {
				label = "Load media"
			}
			ui.ButtonBase(c.Key("load")).Label(label).Padding(4, 8).Border(1, r.Style.Border).Radius(6).
				Disabled(r.Media.Client == nil).Cursor(ui.CursorPointer).Tooltip(source).
				Children(func() { ui.Text(c, label) }).OnClick(func() { *loaded = true })
			return
		}
		if node.Type == "Video" {
			r.videoView(c, node, scope, media)
			return
		}
		switch {
		case media.err != nil:
			r.reportMediaError(media.err)
			ui.Text(c, r.mediaUnavailable()).Label(description).FontSize(12).TextColor(r.Style.Muted)
		case !media.ready:
			ui.Text(c, "Loading…").Label(description).FontSize(12).TextColor(r.Style.Muted)
		case media.image != nil:
			view := ui.Image(c, media.image).Label(description).FillWidth().MaxHeight(240)
			switch r.surface.String(node.Props["fit"], scope) {
			case "contain":
				view.Fit(ui.Contain)
			case "scaleDown":
				view.Fit(ui.ScaleDown)
			case "none":
				view.Fit(ui.NaturalSize)
			case "cover":
				view.Fit(ui.Cover)
			default:
				view.Fit(ui.FillBox)
			}
			switch r.surface.String(node.Props["variant"], scope) {
			case "icon":
				view.Size(24, 24)
			case "avatar":
				view.Size(44, 44).Radius(22).Clip()
			case "smallFeature":
				view.MaxHeight(90)
			case "largeFeature", "header":
				view.MaxHeight(320)
			}
		case media.audio != nil:
			player := media.audio
			r.playButton(c, player.Playing(), player.Play, player.Pause)
			if duration := player.Duration().Seconds(); duration > 0 {
				position := player.Position().Seconds()
				if ui.Slider(c.Key("position"), &position, 0, duration).Label("Playback position").Changed() {
					player.Seek(time.Duration(position * float64(time.Second)))
				}
			}
			if player.Playing() {
				c.After(200 * time.Millisecond)
			}

		}
	})
}

func (r renderer) playButton(c *ui.Context, playing bool, play, pause func()) {
	label, action := "Play", play
	if playing {
		label, action = "Pause", pause
	}
	ui.ButtonBase(c.Key("playback")).Label(label).Padding(4, 8).Disabled(r.Disabled).
		Border(1, r.Style.Border).Radius(6).Children(func() { ui.Text(c, label) }).OnClick(action)
}

type mediaFrame struct {
	ready bool
	image *ui.Bitmap
	audio *audio.Player
	video *ui.VideoPlayer
	err   error
}

func (r renderer) loadMedia(c *ui.Context, element ui.Element, key, kind, source string) mediaFrame {
	slot := ui.Local(element, "a2ui-media", func() *mediaSlot { return new(mediaSlot) })
	if (*slot).key != key || (*slot).state == nil {
		_ = (*slot).Close()
		(*slot).key = key

		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		state := &mediaSource{cancel: cancel}
		services := c.Services()
		go func() {
			r.fetchMedia(ctx, state, kind, source)
			services.Invalidate()
		}()
		(*slot).state = state
	}
	state := (*slot).state
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.video == nil && state.err == nil && (state.decoder != nil || state.videoData != nil) {
		if state.decoder != nil {
			state.video, state.err = ui.NewVideoPlayer(state.decoder)
		} else {
			state.video, state.err = video.OpenData(state.videoData)
			state.videoData = nil
		}
		if state.video != nil {
			state.video.Pause()
		}
	}
	return mediaFrame{state.ready, state.image, state.audio, state.video, state.err}
}

func (r renderer) fetchMedia(ctx context.Context, state *mediaSource, kind, source string) {
	defer state.cancel()
	data, err := mediaBytes(ctx, r.Media.Client, kind, source)
	var bitmap *ui.Bitmap
	var player *audio.Player
	var decoder ui.VideoDecoder
	var videoData []byte
	if err == nil {
		switch kind {
		case "Image":
			var img image.Image
			if _, err = mediaImageConfig(data); err == nil {
				img, _, err = image.Decode(bytes.NewReader(data))
				if err == nil {
					bitmap = ui.NewBitmap(img)
				}
			}
		case "AudioPlayer":
			player, err = audio.OpenStream(bytes.NewReader(data))
		case "Video":
			if r.Media.DecodeVideo != nil {
				decoder, err = r.Media.DecodeVideo(ctx, data)
			} else if bytes.HasPrefix(data, []byte("GIF87a")) || bytes.HasPrefix(data, []byte("GIF89a")) {
				if _, err = mediaImageConfig(data); err == nil {
					decoder, err = ui.NewGIFDecoder(data)
				}
			} else if bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")) {
				if _, err = mediaImageConfig(data); err == nil {
					decoder, err = ui.NewAPNGDecoder(data)
				}
			} else {
				videoData = data
			}
		}
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.closed {
		if player != nil {
			player.Close()
		}
		if closer, ok := decoder.(io.Closer); ok {
			_ = closer.Close()
		}
		return
	}
	state.ready, state.image, state.audio, state.decoder, state.videoData, state.err = true, bitmap, player, decoder, videoData, err
}

func mediaBytes(ctx context.Context, client *http.Client, kind, source string) ([]byte, error) {
	const limit = 16 << 20
	if kind == "Image" && strings.HasPrefix(source, "data:image/") {
		header, body, ok := strings.Cut(source, ",")
		if !ok || !strings.HasSuffix(header, ";base64") || base64.StdEncoding.DecodedLen(len(body)) > limit {
			return nil, fmt.Errorf("invalid A2UI image data URL")
		}
		return base64.StdEncoding.DecodeString(body)
	}
	url, err := a2ui.SafeURL(source)
	if err != nil || (url.Scheme != "http" && url.Scheme != "https") || client == nil {
		return nil, fmt.Errorf("remote A2UI media URL is unavailable")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url.String(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || resp.ContentLength > limit {
		return nil, fmt.Errorf("invalid A2UI media response")
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if len(data) > limit {
		return nil, fmt.Errorf("A2UI media exceeds %d bytes", limit)
	}
	return data, err
}

type mediaSlot struct {
	key   string
	state *mediaSource
}

func (s *mediaSlot) Close() error {
	if s.state == nil {
		return nil
	}
	err := s.state.Close()
	s.state = nil
	return err
}

func (r renderer) videoView(c *ui.Context, node a2ui.Component, scope string, media mediaFrame) {
	player := media.video
	poster := r.surface.String(node.Props["posterUrl"], scope)
	err := media.err
	if player != nil {
		err = player.Err()
	}
	if err != nil {
		r.reportMediaError(err)
		ui.Text(c, r.mediaUnavailable()).FontSize(12).TextColor(r.Style.Muted)
		return
	}
	started := player != nil && (player.Position() > 0 || player.Playing() || player.Ended())
	showPoster := false
	if !started && poster != "" {
		element := ui.Box(c.Key("poster")).FillWidth()
		preview := r.loadMedia(c, element, poster, "Image", poster)
		showPoster = preview.image != nil
		if showPoster {
			element.Children(func() {
				ui.Image(c, preview.image).FillWidth().MaxHeight(230).Fit(ui.Contain)
			})
		}
	}
	if player != nil {
		view := ui.Video(c.Key("picture"), player).FillWidth().MaxHeight(230).Fit(ui.Contain)
		if showPoster {
			// Keep receiving frames while the poster is displayed.
			view.Absolute().Opacity(0)
		}
	}
	if player == nil || !player.Ready() {
		ui.Text(c, "Loading…").FontSize(12).TextColor(r.Style.Muted)
		return
	}
	r.playButton(c, player.Playing(), player.Play, player.Pause)
	if duration := player.Duration().Seconds(); duration > 0 {
		position := player.Position().Seconds()
		if ui.Slider(c.Key("position"), &position, 0, duration).Label("Playback position").Changed() {
			player.Seek(time.Duration(position * float64(time.Second)))
		}
	}
}

func (r renderer) mediaUnavailable() string {
	if r.Media.UnavailableLabel != "" {
		return r.Media.UnavailableLabel
	}
	return "Media unavailable"
}

func (r renderer) reportMediaError(err error) {
	if r.OnError != nil {
		r.OnError(err)
	}
}

func mediaImageConfig(data []byte) (image.Config, error) {
	config, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || config.Width < 1 || config.Height < 1 ||
		config.Width > 16_000_000/config.Height {
		return config, fmt.Errorf("invalid A2UI image dimensions")
	}
	return config, nil
}
