package ui

import (
	"image/color"
	"math"
	"math/rand/v2"

	"charm.land/lipgloss/v2"
)

// Event is a short animation played over the skyline. Draw must be a pure
// function of the frame, so any frame can be rendered (and tested) on its own.
type Event interface {
	Name() string
	Frames() int
	Draw(cv *Canvas, city City, frame int)
}

// EventKinds lists the animations that can play, in a stable order.
var EventKinds = []string{"hero", "swinger", "shooting-star", "ufo", "fireworks"}

var (
	skin        = lipgloss.Color("#f0c8a0")
	capeColor   = lipgloss.Color("#8957e5")
	suitRed     = lipgloss.Color("#ff5f57")
	suitBlue    = lipgloss.Color("#388bfd")
	webColor    = lipgloss.Color("#c9d1d9")
	starWhite   = lipgloss.Color("#ffffff")
	trailDim    = lipgloss.Color("#6e7681")
	ufoBody     = lipgloss.Color("#8b949e")
	ufoDome     = lipgloss.Color("#39c5cf")
	beamColor   = lipgloss.Color("#fff3b0")
	sparkColors = []string{"#ff7b72", "#ffa657", "#e3b341", "#7ee787", "#79c0ff", "#d2a8ff", "#ff9bce"}
)

// NewEvent creates an animation of the given kind, choosing its details
// (which buildings, where in the sky) with rng. It returns nil if the city has
// no room for it, e.g. a hero needs two buildings to jump between.
func NewEvent(kind string, city City, rng *rand.Rand) Event {
	if city.Cols < 10 {
		return nil
	}
	switch kind {
	case "hero":
		return newHeroJump(city, rng)
	case "swinger":
		return newSwinger(city, rng)
	case "shooting-star":
		return newShootingStar(city, rng)
	case "ufo":
		return newUFO(city, rng)
	case "fireworks":
		return newFireworks(city, rng)
	}
	return nil
}

// RandomEvent picks a random animation that fits the city, avoiding a repeat
// of the previous one.
func RandomEvent(city City, rng *rand.Rand, previous string) Event {
	kinds := make([]string, 0, len(EventKinds))
	for _, k := range EventKinds {
		if k != previous {
			kinds = append(kinds, k)
		}
	}
	rng.Shuffle(len(kinds), func(i, j int) { kinds[i], kinds[j] = kinds[j], kinds[i] })
	for _, k := range kinds {
		if e := NewEvent(k, city, rng); e != nil {
			return e
		}
	}
	return nil
}

func lerp(a, b, t float64) float64 { return a + (b-a)*t }

// rooftops returns buildings at least minFloors tall that have room above them
// (no antenna), as building indexes.
func rooftops(city City, minFloors int) []int {
	var out []int
	for i, b := range city.Buildings {
		if b.floors >= minFloors && !b.hasAntenna {
			out = append(out, i)
		}
	}
	return out
}

// ---------- Caped hero: jumps from one rooftop to another ----------

type heroJump struct {
	from, to int // window columns of the two buildings
	height   float64
	flight   int
}

const heroStand = 8 // frames standing before the jump and after landing

func newHeroJump(city City, rng *rand.Rand) Event {
	roofs := rooftops(city, 3)
	if len(roofs) < 2 {
		return nil
	}
	a := roofs[rng.IntN(len(roofs))]
	var candidates []int
	for _, b := range roofs {
		if d := abs(b - a); d >= 3 && d <= 18 {
			candidates = append(candidates, b)
		}
	}
	if len(candidates) == 0 {
		return nil
	}
	b := candidates[rng.IntN(len(candidates))]
	return heroArc(city, city.BuildingX(a), city.BuildingX(b), 3+rng.Float64()*2)
}

// heroArc plans a jump from one roof to another, raising the arc until the
// hero and its trailing cape clear every building on the way.
func heroArc(city City, from, to int, minHeight float64) *heroJump {
	h := &heroJump{from: from, to: to, height: minHeight, flight: max(10, min(26, abs(to-from)))}
	for f := range h.flight {
		t := float64(f+1) / float64(h.flight)
		x, base := h.path(city, t)
		// Highest roof under the hero and the two cells of cape behind it
		limit := city.Rows
		for dx := -2; dx <= 2; dx++ {
			limit = min(limit, city.Roof(x+dx))
		}
		if bump := 4 * t * (1 - t); bump > 0 {
			// need base - height*bump <= limit - 1  →  height >= (base - limit + 1) / bump
			h.height = max(h.height, (base-float64(limit)+1)/bump)
		}
	}
	return h
}

// path returns the hero's column and its straight-line row (before the arc) at t.
func (h *heroJump) path(city City, t float64) (int, float64) {
	y0, y1 := float64(city.Roof(h.from)-2), float64(city.Roof(h.to)-2)
	return int(math.Round(lerp(float64(h.from), float64(h.to), t))), lerp(y0, y1, t)
}

func (h *heroJump) Name() string { return "hero" }
func (h *heroJump) Frames() int  { return heroStand + h.flight + heroStand }

func (h *heroJump) Draw(cv *Canvas, city City, f int) {
	standAt := func(x int) {
		roof := city.Roof(x)
		cv.Set(x, roof-2, 'o', skin)
		cv.Set(x, roof-1, '▲', capeColor)
	}
	switch {
	case f < heroStand:
		standAt(h.from)
	case f < heroStand+h.flight:
		t := float64(f-heroStand+1) / float64(h.flight)
		x, base := h.path(city, t)
		y := max(0, int(math.Floor(base-h.height*4*t*(1-t))))
		dir := 1
		if h.to < h.from {
			dir = -1
		}
		cape := '='
		if f%2 == 0 {
			cape = '~'
		}
		cv.Set(x, y, 'o', skin)
		for i := 1; i <= 2; i++ { // the cape trails behind, tucking in against walls
			if cx := x - i*dir; y < city.Roof(cx) {
				cv.Set(cx, y, cape, capeColor)
			}
		}
	default:
		standAt(h.to)
	}
}

// ---------- Web swinger: swings across the city on lines ----------

type swinger struct {
	anchors  []int // x of each anchor, left to right
	low      int   // lowest row of the swing
	perSwing int
}

func newSwinger(city City, rng *rand.Rand) Event {
	span := 10 + rng.IntN(6)
	start := rng.IntN(max(1, city.Cols/3))
	var anchors []int
	for x := start; x+span < city.Cols && len(anchors) < 4; x += span {
		anchors = append(anchors, x)
	}
	if len(anchors) < 2 {
		return nil
	}
	return &swinger{anchors: anchors, low: city.Rows - maxFloors/2 - 2, perSwing: 12}
}

func (s *swinger) Name() string { return "swinger" }
func (s *swinger) Frames() int  { return (len(s.anchors) - 1) * s.perSwing }

func (s *swinger) Draw(cv *Canvas, city City, f int) {
	seg := min(f/s.perSwing, len(s.anchors)-2)
	t := float64(f-seg*s.perSwing) / float64(s.perSwing)
	x0, x1 := float64(s.anchors[seg]), float64(s.anchors[seg+1])
	top := float64(skyRows + 1)
	// A pendulum: high at both ends, lowest in the middle under the anchor
	x := int(math.Round(lerp(x0, x1, t)))
	y := int(math.Round(top + (float64(s.low)-top)*4*t*(1-t)))
	ax := int(math.Round((x0 + x1) / 2))
	line(cv, ax, 0, x, y-1, webColor)
	cv.Set(x, y, 'o', suitRed)
	cv.Set(x, y+1, 'x', suitBlue)
}

// line draws a straight line with characters that follow its slope.
func line(cv *Canvas, x0, y0, x1, y1 int, fg color.Color) {
	dx, dy := x1-x0, y1-y0
	ch := '|'
	switch {
	case abs(dy) < abs(dx)/3:
		ch = '-'
	case abs(dx) >= abs(dy)/2 && (dx > 0) == (dy > 0):
		ch = '\\'
	case abs(dx) >= abs(dy)/2:
		ch = '/'
	}
	steps := max(abs(dx), abs(dy))
	for i := 0; i <= steps; i++ {
		t := 0.0
		if steps > 0 {
			t = float64(i) / float64(steps)
		}
		cv.Set(int(math.Round(lerp(float64(x0), float64(x1), t))), int(math.Round(lerp(float64(y0), float64(y1), t))), ch, fg)
	}
}

// ---------- Shooting star ----------

type shootingStar struct {
	x, y   int
	length int
}

func newShootingStar(city City, rng *rand.Rand) Event {
	return &shootingStar{x: rng.IntN(max(1, city.Cols/2)), y: rng.IntN(2), length: 9 + rng.IntN(6)}
}

func (s *shootingStar) Name() string { return "shooting-star" }
func (s *shootingStar) Frames() int  { return s.length + 4 } // + time for the streak to catch up

func (s *shootingStar) Draw(cv *Canvas, city City, f int) {
	const speedX, speedY = 2.5, 0.5 // cells per frame: fast and shallow
	streak := []struct {
		ch rune
		fg color.Color
	}{{'*', starWhite}, {'-', webColor}, {'-', webColor}, {'-', trailDim}, {'-', trailDim}, {'.', trailDim}, {'.', trailDim}}
	for k, seg := range streak {
		p := float64(f) - float64(k)*0.5 // each trail cell lags half a frame behind the one before
		if p < 0 || p > float64(s.length) {
			continue
		}
		x := s.x + int(math.Round(p*speedX))
		y := s.y + int(math.Round(p*speedY))
		if y >= city.Roof(x) { // burns out against a building
			continue
		}
		cv.Set(x, y, seg.ch, seg.fg)
	}
}

// ---------- UFO: crosses the sky and beams a building ----------

type ufo struct {
	target int // window column it hovers over
	cols   int
}

const (
	ufoSpeed = 2
	ufoHover = 18
	ufoWidth = 5
)

func newUFO(city City, rng *rand.Rand) Event {
	roofs := rooftops(city, 2)
	if len(roofs) == 0 {
		return nil
	}
	return &ufo{target: city.BuildingX(roofs[rng.IntN(len(roofs))]), cols: city.Cols}
}

func (u *ufo) Name() string { return "ufo" }
func (u *ufo) arrive() int  { return (u.target + ufoWidth) / ufoSpeed }
func (u *ufo) Frames() int  { return u.arrive() + ufoHover + (u.cols-u.target)/ufoSpeed + ufoWidth }

func (u *ufo) Draw(cv *Canvas, city City, f int) {
	var x int
	switch arrive := u.arrive(); {
	case f < arrive:
		x = f*ufoSpeed - ufoWidth
	case f < arrive+ufoHover:
		x = u.target
		// beam from under the saucer down to the roof, pulsing
		ch := '│'
		if f%4 < 2 {
			ch = '┃'
		}
		for y := 2; y < city.Roof(u.target); y++ {
			cv.Set(u.target, y, ch, beamColor)
		}
	default:
		x = u.target + (f-arrive-ufoHover)*ufoSpeed
	}
	// saucer centered on x
	lightsOn := f%2 == 0
	edge := '-'
	if lightsOn {
		edge = '*'
	}
	cv.Set(x-2, 1, edge, ufoBody)
	cv.Text(x-1, 1, "=O=", ufoBody)
	cv.Set(x, 1, 'O', ufoDome)
	cv.Set(x+2, 1, edge, ufoBody)
}

// ---------- Fireworks over the tallest building ----------

type rocket struct {
	x, burstY, delay int
	color            string
}

type fireworks struct{ rockets []rocket }

const (
	rocketRise = 7
	burstLife  = 9
)

func newFireworks(city City, rng *rand.Rand) Event {
	best := -1
	for i, b := range city.Buildings {
		if b.hasAntenna {
			best = i
		}
	}
	center := city.Cols / 2
	if best >= 0 {
		center = city.BuildingX(best)
	}
	var rs []rocket
	palette := rng.Perm(len(sparkColors)) // a different color for each rocket
	for i := range 3 {
		x := center + (i-1)*(6+rng.IntN(4))
		if x < 2 || x >= city.Cols-2 {
			continue
		}
		rs = append(rs, rocket{
			x: x, burstY: 1 + rng.IntN(2), delay: i * 5,
			color: sparkColors[palette[i]],
		})
	}
	if len(rs) == 0 {
		return nil
	}
	return &fireworks{rockets: rs}
}

func (fw *fireworks) Name() string { return "fireworks" }
func (fw *fireworks) Frames() int {
	last := 0
	for _, r := range fw.rockets {
		last = max(last, r.delay+rocketRise+burstLife)
	}
	return last
}

func (fw *fireworks) Draw(cv *Canvas, city City, f int) {
	for _, r := range fw.rockets {
		t := f - r.delay
		switch {
		case t < 0:
		case t < rocketRise:
			start := city.Roof(r.x) - 1
			y := int(math.Round(lerp(float64(start), float64(r.burstY), float64(t+1)/rocketRise)))
			cv.Set(r.x, y, '|', webColor)
		case t < rocketRise+burstLife:
			age := t - rocketRise
			radius := 1 + float64(age)*0.6
			ch, col := '*', lipgloss.Color(r.color)
			if age > burstLife*2/3 {
				ch, col = '.', trailDim
			} else if age > burstLife/3 {
				ch = '+'
			}
			for a := 0.0; a < 2*math.Pi; a += math.Pi / 6 {
				x := r.x + int(math.Round(math.Cos(a)*radius*2)) // cells are ~twice as tall as wide
				y := r.burstY + int(math.Round(math.Sin(a)*radius))
				if y < city.Roof(x) {
					cv.Set(x, y, ch, col)
				}
			}
		}
	}
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
