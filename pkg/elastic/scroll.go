// Package elastic adds damped edge scrolling to native MyGo scroll views.
package elastic

import (
	"math"
	"time"

	"github.com/egoist/mygo/ui"
)

// Offset is the visual displacement of scroll content, in DIPs.
type Offset struct{ X, Y float32 }

// Scroll builds a vertical scroll viewport and returns the displacement to
// apply to its contents. Keep the viewport, its clip and its scrollbars fixed.
func Scroll(c *ui.Context) (ui.Element, Offset) {
	return attach(c, ui.Scroll(c), false, true)
}

// Horizontal builds a horizontal scroll viewport.
func Horizontal(c *ui.Context) (ui.Element, Offset) {
	return attach(c, ui.ScrollHorizontal(c), true, false)
}

// List retains MyGo's virtualized rows, focus and follow-end behavior.
func List(c *ui.Context, state *ui.ListState, count int, row func(int, Offset)) ui.Element {
	// List builds its rows before returning its element. Keep the effect on
	// each row's content so virtual rows keep their original layout positions.
	slot := ui.Local(ui.Box(c.Key("list-elastic-state")).Absolute().Size(0, 0),
		"scroll", func() scrollState { return scrollState{} })
	now := c.Now()
	offset := slot.offset(c)
	list := ui.List(c, state, count, func(i int) { row(i, offset) })
	bind(c, list, slot, now, false, true)
	return list
}

// Apply moves the contents while leaving the viewport and its scrollbar fixed.
func (offset Offset) Apply(content ui.Element) ui.Element {
	if offset.X != 0 || offset.Y != 0 {
		content.Translate(offset.X, offset.Y)
	}
	return content
}

const (
	gestureIdle = 48 * time.Millisecond
	wheelIdle   = 24 * time.Millisecond
	// A critically damped spring, with time measured in seconds.
	springFrequency = 19
	rubberbandScale = .55
)

type axis struct {
	pull, velocity float64
	at, release    time.Time
	momentumEdge   int
}

func (a *axis) value(now time.Time) float64 {
	t := now.Sub(a.release).Seconds()
	if t <= 0 {
		return a.pull
	}
	// The closed-form spring starts at the displayed pull and carries its
	// velocity through release, independent of the frame rate.
	decay := math.Exp(-springFrequency * t)
	speed := a.velocity + springFrequency*a.pull
	value := (a.pull + speed*t) * decay
	velocity := (a.velocity - springFrequency*speed*t) * decay
	if value*a.pull < 0 || math.Abs(value) < .15 && math.Abs(velocity) < 3 {
		return 0
	}
	return value
}

func (a *axis) move(now time.Time, offset, reach, delta, viewport float32, event ui.InputEvent) (float32, bool) {
	if reach <= 0 || viewport <= 0 {
		*a = axis{}
		return offset, false
	}
	pull := a.value(now)
	if pull < 0 && offset > .5 || pull > 0 && offset < reach-.5 {
		pull = 0
	}
	// Platform momentum drives normal scrolling. Once it reaches an
	// edge, the spring owns the rest of that outward motion.
	if event.Momentum && (a.momentumEdge < 0 && offset <= .5 && delta < 0 ||
		a.momentumEdge > 0 && offset >= reach-.5 && delta > 0) {
		return offset, true
	}

	target := float64(offset) + float64(delta)
	if pull*float64(delta) < 0 {
		// Reverse against the visible displacement before moving inside
		// the bounds. Both movements meet at exactly the same position.
		target += pull
		if (pull < 0 && target < 0) || (pull > 0 && target > float64(reach)) {
			a.pull = target - math.Max(0, math.Min(target, float64(reach)))
			a.velocity = 0
			a.at, a.release = now, now.Add(inputHold(event))
			a.momentumEdge = 0
			return offset, true
		}
	}

	clamped := math.Max(0, math.Min(target, float64(reach)))
	overflow := target - clamped
	if overflow == 0 {
		*a = axis{at: now}
		return float32(clamped), true
	}
	// Use the actual viewport as the reference, with a ceiling so a
	// large desktop window cannot stretch excessively.
	dimension := min(float64(viewport), 200)
	raw := math.Abs(overflow)
	if pull*overflow > 0 {
		visible := min(math.Abs(pull), dimension*.999)
		raw += dimension * visible / (rubberbandScale * (dimension - visible))
	}
	next := math.Copysign(dimension*raw/(dimension/rubberbandScale+raw), overflow)
	dt := now.Sub(a.at)
	if dt <= 0 || dt > gestureIdle {
		dt = time.Second / 60
		if !event.Precise {
			dt = wheelIdle
		}
	}
	velocity := (next - pull) / max(dt.Seconds(), 1.0/240)
	if !event.Momentum && (a.at.IsZero() || now.Sub(a.at) > gestureIdle || !event.Precise) {
		// A standalone delta has no reliable release velocity.
		velocity = 0
	} else if now.Sub(a.at) <= gestureIdle && pull*next > 0 {
		// Keep small variations in input delivery from jolting the spring.
		velocity = .6*velocity + .4*a.velocity
	}
	limit := min(1600, (dimension-math.Abs(next))*springFrequency)
	velocity = math.Copysign(min(math.Abs(velocity), limit), velocity)
	*a = axis{pull: next, velocity: velocity, at: now, release: now.Add(inputHold(event))}
	if event.Momentum {
		a.momentumEdge = 1
		if next < 0 {
			a.momentumEdge = -1
		}
		a.release = now
	}
	return float32(clamped), true
}

type scrollState struct {
	track    ui.ScrollState
	x, y     axis
	viewport ui.Rect
}

func attach(c *ui.Context, view ui.Element, horizontal, vertical bool) (ui.Element, Offset) {
	slot := ui.Local(view, "elastic-scroll", func() scrollState { return scrollState{} })
	bind(c, view, slot, c.Now(), horizontal, vertical)
	return view, slot.offset(c)
}

func (s *scrollState) offset(c *ui.Context) Offset {
	if c.Preferences().ReduceMotion {
		s.x, s.y = axis{}, axis{}
		return Offset{}
	}
	x, y := s.x.value(c.Now()), s.y.value(c.Now())
	if s.track.MaxX <= 0 || x < 0 && s.track.X > .5 || x > 0 && s.track.X < s.track.MaxX-.5 {
		s.x, x = axis{}, 0
	}
	if s.track.MaxY <= 0 || y < 0 && s.track.Y > .5 || y > 0 && s.track.Y < s.track.MaxY-.5 {
		s.y, y = axis{}, 0
	}
	if x != 0 || y != 0 {
		c.AnimationFrame()
	}
	x = -x
	if c.Direction() == ui.RightToLeft {
		x = -x
	}
	return Offset{X: float32(x), Y: -float32(y)}
}

func bind(c *ui.Context, view ui.Element, state *scrollState, now time.Time, horizontal, vertical bool) {
	state.viewport = view.LayoutBounds()
	view.TrackScroll(&state.track)
	reduceMotion := c.Preferences().ReduceMotion
	rtl := c.Direction() == ui.RightToLeft
	view.HandleInput(func(ev ui.InputEvent) bool {
		if ev.Kind != ui.InputScroll || reduceMotion || ev.Mods&ui.Ctrl != 0 {
			return false
		}
		dx, dy := ev.DX, ev.DY
		if dx == 0 && ev.Mods&ui.Shift != 0 {
			dx, dy = dy, 0
			if rtl {
				dx = -dx
			}
		}
		if rtl {
			dx = -dx
		}
		// Leave a gesture aimed at another axis to native nested scrollers,
		// such as a horizontal Markdown table inside a vertical transcript.
		if ev.Precise && ((!horizontal && math.Abs(float64(dx)) > math.Abs(float64(dy))) ||
			(!vertical && math.Abs(float64(dy)) > math.Abs(float64(dx)))) {
			return false
		}
		if !horizontal {
			dx = 0
		}
		if !vertical {
			dy = 0
		}
		if dx == 0 && dy == 0 {
			return false
		}
		at := ev.Time
		if at.IsZero() {
			at = now
		}
		var x, y bool
		if dx != 0 {
			state.track.X, x = state.x.move(at, state.track.X, state.track.MaxX, dx, state.viewport.W, ev)
		}
		if dy != 0 {
			state.track.Y, y = state.y.move(at, state.track.Y, state.track.MaxY, dy, state.viewport.H, ev)
		}
		return x || y
	})
}

func inputHold(event ui.InputEvent) time.Duration {
	if event.Precise && !event.Momentum {
		return gestureIdle
	}
	return wheelIdle
}
