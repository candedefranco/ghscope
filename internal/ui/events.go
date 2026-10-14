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
	heroSuit    = lipgloss.Color("#454e5e")
	capeColor   = lipgloss.Color("#8957e5")
	capeShade   = lipgloss.Color("#6e40c9")
	beltColor   = lipgloss.Color("#e3b341")
	suitRed     = lipgloss.Color("#e5534b")
	suitBlue    = lipgloss.Color("#316dca")
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

var heroPalette = map[byte]color.Color{'k': heroSuit, 'c': capeColor, 'd': capeShade, 's': skin, 'y': beltColor, 'w': starWhite}

var (
	heroStanding = NewSprite(heroPalette,
		".k.k.",
		".kkk.",
		".wkw.",
		"ckskc",
		"ckykc",
		"dk.kd",
	)
	heroCrouching = NewSprite(heroPalette,
		".k.k.",
		".kkk.",
		".wkw.",
		"ckykc",
		"dkdkd",
	)
	// In the air the cape opens like wings; two frames make its edge ripple.
	heroGliding = [2]Sprite{
		NewSprite(heroPalette,
			"...k.k...",
			"c..kkk..c",
			"cc.wkw.cc",
			"ccckykccc",
			"d.dk.kd.d",
		),
		NewSprite(heroPalette,
			"...k.k...",
			"c..kkk..c",
			"cc.wkw.cc",
			"ccckykccc",
			".d.k.k.d.",
		),
	}
)

const (
	heroStand  = 8 // frames on a roof before the jump and after landing
	heroCrouch = 2 // of those, frames crouched right before and after the flight
	heroBody   = 1 // columns either side of the hero's center that must clear roofs
)

type heroJump struct {
	from, to int     // window columns of the two buildings
	height   float64 // how far the arc rises above the straight line, in pixels
	flight   int
}

// feet returns the pixel row just above the roof at column x.
func feet(city City, x int) int { return 2*city.Roof(x) - 1 }

// drawOn draws a sprite by its bottom center, hiding pixels that would land
// inside a building, so a cape tucks in behind a wall instead of covering it.
func drawOn(cv *Canvas, city City, s Sprite, x, feet int) {
	s.Draw(cv, x-s.W()/2, feet-s.H()+1, func(x, py int) bool { return py < 2*city.Roof(x) })
}

// heroRoofs returns buildings low enough for the whole hero to fit under the
// top of the sky. With clear set, only those without a taller neighbour that
// would hide part of the hero.
func heroRoofs(city City, clear bool) []int {
	var out []int
	for _, i := range rooftops(city, 2) {
		x := city.BuildingX(i)
		ok := 2*city.Roof(x) >= heroStanding.H()
		for dx := -heroStanding.W() / 2; clear && dx <= heroStanding.W()/2; dx++ {
			ok = ok && city.Roof(x+dx) >= city.Roof(x)
		}
		if ok {
			out = append(out, i)
		}
	}
	return out
}

// newHeroJump picks two roofs a few buildings apart, preferring clear ones.
func newHeroJump(city City, rng *rand.Rand) Event {
	for _, roofs := range [][]int{heroRoofs(city, true), heroRoofs(city, false)} {
		if len(roofs) < 2 {
			continue
		}
		a := roofs[rng.IntN(len(roofs))]
		var candidates []int
		for _, b := range roofs {
			if d := abs(b - a); d >= 4 && d <= 18 {
				candidates = append(candidates, b)
			}
		}
		if len(candidates) > 0 {
			b := candidates[rng.IntN(len(candidates))]
			return heroArc(city, city.BuildingX(a), city.BuildingX(b), 6+rng.Float64()*4)
		}
	}
	return nil
}

// heroArc plans a jump from one roof to another, raising the arc until the
// hero's body clears every building on the way, but never so high that the
// hero leaves the sky: a tower too tall to clear is flown behind instead. (The
// tips of the cape may pass behind buildings too.)
func heroArc(city City, from, to int, minHeight float64) *heroJump {
	h := &heroJump{from: from, to: to, flight: max(10, min(26, abs(to-from)))}
	need, ceiling := minHeight, math.Inf(1)
	for f := range h.flight {
		t := float64(f+1) / float64(h.flight)
		bump := 4 * t * (1 - t)
		if bump <= 0 {
			continue
		}
		// The hero's feet are at base - height*bump.
		x, base := h.line(city, t)
		limit := city.Rows // highest roof under the hero's body
		for dx := -heroBody; dx <= heroBody; dx++ {
			limit = min(limit, city.Roof(x+dx))
		}
		need = max(need, (base-float64(2*limit-1))/bump)                  // feet above that roof
		ceiling = min(ceiling, (base-float64(heroGliding[0].H()-1))/bump) // head below the top
	}
	h.height = max(0, min(need, ceiling))
	return h
}

// line returns the hero's column and feet row at t (0..1) on the straight
// line between the two roofs; the arc lifts it above that.
func (h *heroJump) line(city City, t float64) (int, float64) {
	x := int(math.Round(lerp(float64(h.from), float64(h.to), t)))
	return x, lerp(float64(feet(city, h.from)), float64(feet(city, h.to)), t)
}

// pos returns the hero's column and feet row at frame f of the flight.
func (h *heroJump) pos(city City, f int) (x, feetY int) {
	t := float64(f-heroStand+1) / float64(h.flight)
	x, base := h.line(city, t)
	return x, int(math.Floor(base - h.height*4*t*(1-t)))
}

func (h *heroJump) Name() string { return "hero" }
func (h *heroJump) Frames() int  { return heroStand + h.flight + heroStand }

func (h *heroJump) Draw(cv *Canvas, city City, f int) {
	land := heroStand + h.flight
	switch {
	case f < heroStand-heroCrouch:
		drawOn(cv, city, heroStanding, h.from, feet(city, h.from))
	case f < heroStand:
		drawOn(cv, city, heroCrouching, h.from, feet(city, h.from))
	case f < land:
		x, y := h.pos(city, f)
		drawOn(cv, city, heroGliding[f/2%2], x, y)
	case f < land+heroCrouch:
		drawOn(cv, city, heroCrouching, h.to, feet(city, h.to))
	default:
		drawOn(cv, city, heroStanding, h.to, feet(city, h.to))
	}
}

// ---------- Web swinger: swings across the city on lines ----------

var swingerPalette = map[byte]color.Color{'r': suitRed, 'b': suitBlue, 'w': starWhite}

// pose is a swinger sprite and the column of the hand holding the line, on
// its top row.
type pose struct {
	Sprite
	hand int
}

var (
	// Reaching ahead for the anchor on the way down, the other arm and the
	// legs flung out behind...
	swingReach = pose{NewSprite(swingerPalette,
		"........r",
		"...rrr..r",
		"...wrw.r.",
		"...rrr.r.",
		".rrbrbr..",
		"...brb...",
		"..bb.bb..",
		".rr...rr.",
	), 8}
	// ...and hanging under it on the way back up.
	swingHang = pose{NewSprite(swingerPalette,
		"......r.",
		"..rrr.r.",
		"..wrw.r.",
		"..rrr.r.",
		".rbrbr..",
		"..brb...",
		"..b.b...",
		".rr.rr..",
	), 6}
)

type swinger struct {
	anchors  []int // x of each anchor, left to right
	top, low int   // pixel row of the hand at the ends and at the bottom of a swing
	perSwing int
}

func newSwinger(city City, rng *rand.Rand) Event {
	span := 10 + rng.IntN(6)
	start := rng.IntN(max(1, city.Cols/3))
	var anchors []int
	for x := start; x+span < city.Cols && len(anchors) < 4; x += span {
		anchors = append(anchors, x)
	}
	top, low := 2, 2*city.Rows-swingReach.H()-2 // the feet stay above the street
	if len(anchors) < 2 || low-top < 4 {
		return nil
	}
	return &swinger{anchors: anchors, top: top, low: low, perSwing: 12}
}

func (s *swinger) Name() string { return "swinger" }
func (s *swinger) Frames() int  { return (len(s.anchors) - 1) * s.perSwing }

func (s *swinger) Draw(cv *Canvas, city City, f int) {
	seg := min(f/s.perSwing, len(s.anchors)-2)
	t := float64(f-seg*s.perSwing) / float64(s.perSwing)
	x0, x1 := float64(s.anchors[seg]), float64(s.anchors[seg+1])
	// A pendulum: the hand is high at both ends, lowest in the middle under the anchor
	x := int(math.Round(lerp(x0, x1, t)))
	y := int(math.Round(lerp(float64(s.top), float64(s.low), 4*t*(1-t))))
	y -= y % 2 // the hand on a cell's top half, so the web can reach it
	ax := int(math.Round((x0 + x1) / 2))
	p := swingReach
	if t >= 0.5 {
		p = swingHang
	}
	thread(cv, 2*ax+1, 0, 2*x+1, 2*y-1, webColor)
	p.Draw(cv, x-p.hand, y, nil)
}

// thread draws a fine line in braille dots, which split each cell into 2×4,
// so it stays thin and continuous at any angle. Coordinates are in dots.
func thread(cv *Canvas, x0, y0, x1, y1 int, fg color.Color) {
	bits := [2][4]rune{{0x01, 0x02, 0x04, 0x40}, {0x08, 0x10, 0x20, 0x80}}
	cells := map[[2]int]rune{}
	steps := max(abs(x1-x0), abs(y1-y0))
	for i := 0; i <= steps; i++ {
		t := 0.0
		if steps > 0 {
			t = float64(i) / float64(steps)
		}
		x := int(math.Round(lerp(float64(x0), float64(x1), t)))
		y := int(math.Round(lerp(float64(y0), float64(y1), t)))
		if x >= 0 && y >= 0 {
			cells[[2]int{x / 2, y / 4}] |= bits[x%2][y%4]
		}
	}
	for c, dots := range cells {
		cv.Set(c[0], c[1], 0x2800+dots, fg)
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
