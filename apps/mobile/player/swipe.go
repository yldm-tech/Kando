package player

import (
	"time"

	"kando/apps/mobile/locales"
	"kando/apps/mobile/look"

	"github.com/nsgo-dev/nsgo/core"
)

// The swipe between films: how far of the screen's height, or how fast, a drag goes for the next film to take the current one's place; how much a drag moves the picture at either end of the list, which springs back; and how long the page takes to slide.
const (
	swipeShare    = 0.2
	swipeVelocity = 800
	swipeRubber   = 0.25
	swipePage     = 220 * time.Millisecond
)

// swiper goes on through the list the film was opened from (Layer.SetFeed) as a short-video feed does, while on (a phone's upright full screen, the list of lines put away): a drag up on the picture slides it out the top as the next film's poster comes in from the foot and plays the next film, a drag down the one before; at either end the picture gives a little and springs back. The next film plays where it was left, with its own loop and no recap, the app's settings kept (Layer.goOn). Its actions offer the same to assistive technology.
type swiper struct {
	cx *core.Scope
	l  *Layer
	p  *play
	on func() bool
	// drag is how far the picture is moved, in points, up for less than 0.
	drag *core.AnimatedValue
	// next and prev are the films a swipe goes on to, "" for none.
	next, prev *core.Memo[string]
	gone       bool
}

func newSwiper(cx *core.Scope, l *Layer, p *play, key string, on func() bool) *swiper {
	return &swiper{cx: cx, l: l, p: p, on: on, drag: core.NewAnimatedValue(cx, 0),
		next: core.NewMemo(cx, func() string { return l.neighbor(key, 1) }),
		prev: core.NewMemo(cx, func() string { return l.neighbor(key, -1) })}
}

// pan follows a drag on the picture.
func (w *swiper) pan(e core.PanEvent) {
	if w.gone {
		return
	}
	if !core.Untracked(w.cx, w.on) {
		w.drag.Set(0)
		return
	}
	next, prev := w.next.GetUntracked(), w.prev.GetUntracked()
	dy := float64(e.Translation.Y)
	switch e.Phase {
	case core.GestureEnded:
		height := float64(w.cx.Env().Window().Height)
		switch {
		case next != "" && (dy <= -height*swipeShare || float64(e.Velocity.Y) <= -swipeVelocity):
			w.page(next, -height)
		case prev != "" && (dy >= height*swipeShare || float64(e.Velocity.Y) >= swipeVelocity):
			w.page(prev, height)
		default:
			w.back()
		}
	case core.GestureCancelled:
		w.back()
	default:
		if (dy < 0 && next == "") || (dy > 0 && prev == "") {
			dy *= swipeRubber
		}
		w.drag.Set(dy)
	}
}

// back springs the picture back to its place.
func (w *swiper) back() {
	w.cx.StartAnimation(core.Timing(w.drag, 0, core.TimingConfig{Duration: swipePage, Easing: core.BezierEasing(core.EaseOut)}), nil)
}

// page slides the picture away to to and plays key in its place, once the slide has run, or at once for less motion.
func (w *swiper) page(key string, to float64) {
	w.gone = true
	done := false
	goOn := func() {
		if !done {
			done = true
			w.l.goOn(w.p, key)
		}
	}
	if w.cx.Env().ReduceMotion() {
		goOn()
		return
	}
	w.cx.StartAnimation(core.Timing(w.drag, to, core.TimingConfig{Duration: swipePage, Easing: core.BezierEasing(core.EaseOut)}), func(bool) { goOn() })
	// Some platforms report no animation's end.
	w.cx.AfterFunc(swipePage+40*time.Millisecond, goOn)
}

// The names of the swipe's actions for assistive technology.
const (
	actionNextFilm = "nextFilm"
	actionPrevFilm = "prevFilm"
)

// actions are the next and the film before, while a swipe would go to them.
func (w *swiper) actions() []core.AccessibilityAction {
	if !w.on() {
		return nil
	}
	var out []core.AccessibilityAction
	if w.next.Get() != "" {
		out = append(out, core.AccessibilityAction{Name: actionNextFilm, Label: locales.PlayerNextFilm(w.cx)})
	}
	if w.prev.Get() != "" {
		out = append(out, core.AccessibilityAction{Name: actionPrevFilm, Label: locales.PlayerPrevFilm(w.cx)})
	}
	return out
}

// act runs an action of actions.
func (w *swiper) act(name string) {
	key := w.next.GetUntracked()
	if name == actionPrevFilm {
		key = w.prev.GetUntracked()
	}
	if key != "" && !w.gone {
		w.gone = true
		w.l.goOn(w.p, key)
	}
}

// neighbors are the posters of the next film under the picture's foot and of the one before over its top, which the drag moves with the picture, so the next slides in as the picture slides out.
func (w *swiper) neighbors(cx *core.Scope) core.Node {
	poster := func(id string, k *core.Memo[string], top core.Length) core.Node {
		return core.Show{When: func() bool { return w.on() && k.Get() != "" }, Then: func(cx *core.Scope) core.Node {
			return core.View{TestID: id, PointerEvents: core.PointerEventsNone, AccessibilityElementsHidden: func() bool { return true },
				Style: core.Style{Position: core.Absolute, Left: core.Pt(0), Right: core.Pt(0), Top: top, Height: core.Pct(100), Background: core.RGB(0x000000), Animated: &core.AnimatedStyle{TranslateY: w.drag}},
				Children: []core.Node{core.Image{ResizeMode: core.Contain, ElementsHidden: true, Style: fill, SourceFunc: func() core.ImageSource {
					f, _ := w.p.s.Film(k.Get())
					return look.LocalPoster(w.p.s, f)
				}}}}
		}}
	}
	return core.Group([]core.Node{poster("player-swipe-next", w.next, core.Pct(100)), poster("player-swipe-prev", w.prev, core.Pct(-100))})
}
