package ui

import (
	"fmt"
	"math/rand/v2"
	"testing"
)

func seeded(n int) *rand.Rand { return rand.New(rand.NewPCG(uint64(n), 99)) }

// drawn returns the cells an event changes on top of the plain city.
func drawn(city City, ev Event, frame int) map[[2]int]rune {
	base := NewCanvas(city.Cols, city.Rows)
	drawCity(base, city, 1)
	cv := NewCanvas(city.Cols, city.Rows)
	drawCity(cv, city, 1)
	ev.Draw(cv, city, frame)
	out := map[[2]int]rune{}
	for y := range city.Rows {
		for x := range city.Cols {
			if cv.At(x, y) != base.At(x, y) {
				out[[2]int{x, y}] = cv.At(x, y)
			}
		}
	}
	return out
}

func realCity() City { return NewCity(fakeUser(0).Contributions.Calendar, 106, maxFloors) }

// towerCity has a skyscraper between two short buildings.
func towerCity() City {
	floors := []int{3, 3, 0, 0, maxFloors - 1, 0, 0, 3, 3}
	city := City{Rows: skyRows + maxFloors, Cols: len(floors) * buildingW}
	for _, f := range floors {
		city.Buildings = append(city.Buildings, building{floors: f, litShare: 0.5})
	}
	return city
}

func TestEveryEventAnimates(t *testing.T) {
	city := realCity()
	for _, kind := range EventKinds {
		t.Run(kind, func(t *testing.T) {
			for seed := range 20 {
				ev := NewEvent(kind, city, seeded(seed))
				if ev == nil {
					t.Fatalf("seed %d: no room for %s in a full year", seed, kind)
				}
				if ev.Name() != kind {
					t.Errorf("Name() = %q", ev.Name())
				}
				n := ev.Frames()
				if n < 5 || n > 200 {
					t.Fatalf("seed %d: %d frames", seed, n)
				}
				visible := 0
				for f := range n {
					if len(drawn(city, ev, f)) > 0 {
						visible++
					}
				}
				if visible < n/2 {
					t.Errorf("seed %d: only %d of %d frames show anything", seed, visible, n)
				}
			}
		})
	}
}

func TestEventsAreDeterministicPerFrame(t *testing.T) {
	city := realCity()
	for _, kind := range EventKinds {
		ev := NewEvent(kind, city, seeded(3))
		a, b := NewCanvas(city.Cols, city.Rows), NewCanvas(city.Cols, city.Rows)
		ev.Draw(a, city, 5)
		ev.Draw(b, city, 5)
		if a.Render() != b.Render() {
			t.Errorf("%s: drawing the same frame twice gave different results", kind)
		}
	}
}

// Everything except the swinger (who swings in front of buildings) stays out of them.
func TestEventsDontDrawInsideBuildings(t *testing.T) {
	for _, city := range []City{realCity(), towerCity()} {
		for _, kind := range []string{"hero", "shooting-star", "ufo", "fireworks"} {
			for seed := range 30 {
				ev := NewEvent(kind, city, seeded(seed))
				if ev == nil {
					continue
				}
				for f := range ev.Frames() {
					for pos, ch := range drawn(city, ev, f) {
						if x, y := pos[0], pos[1]; y >= city.Roof(x) {
							t.Fatalf("%s seed %d frame %d: %q drawn inside a building at (%d,%d), roof %d",
								kind, seed, f, ch, x, y, city.Roof(x))
						}
					}
				}
			}
		}
	}
}

func TestHeroJumpsOverASkyscraper(t *testing.T) {
	city := towerCity()
	hero := heroArc(city, city.BuildingX(0), city.BuildingX(8), 1)
	for f := heroStand; f < heroStand+hero.flight; f++ {
		x, feet := hero.pos(city, f)
		if top := feet - heroGliding[0].H() + 1; top < 0 {
			t.Errorf("frame %d: the hero's head is off the top of the sky (row %d)", f, top)
		}
		for dx := -heroBody; dx <= heroBody; dx++ {
			if feet >= 2*city.Roof(x+dx) {
				t.Errorf("frame %d: the hero's body goes into a building at column %d", f, x+dx)
			}
		}
	}
	last := drawn(city, hero, hero.Frames()-1)
	to := city.BuildingX(8)
	if _, ok := last[[2]int{to - 1, city.Roof(to) - 1}]; !ok { // a foot
		t.Errorf("the hero should end standing on the target roof, got %v", last)
	}
}

// A tower too tall to clear without leaving the sky is flown behind.
func TestHeroStaysInTheSky(t *testing.T) {
	city := towerCity()
	city.Buildings[4].floors = maxFloors + skyRows - 1
	hero := heroArc(city, city.BuildingX(0), city.BuildingX(8), 1)
	for f := heroStand; f < heroStand+hero.flight; f++ {
		if _, feet := hero.pos(city, f); feet-heroGliding[0].H()+1 < 0 {
			t.Fatalf("frame %d: the hero left the sky", f)
		}
	}
}

func TestRandomEventAvoidsRepeats(t *testing.T) {
	city := realCity()
	for seed := range 50 {
		for _, prev := range EventKinds {
			if ev := RandomEvent(city, seeded(seed), prev); ev == nil || ev.Name() == prev {
				t.Fatalf("seed %d after %s: got %v", seed, prev, ev)
			}
		}
	}
}

func TestEventsNeedRoom(t *testing.T) {
	tiny := City{Rows: skyRows + maxFloors, Cols: 6, Buildings: make([]building, 3)}
	for _, kind := range EventKinds {
		if ev := NewEvent(kind, tiny, seeded(1)); ev != nil {
			t.Errorf("%s should not play in a 6-column city", kind)
		}
	}
	if ev := NewEvent("dragon", realCity(), seeded(1)); ev != nil {
		t.Error("unknown kinds should return nil")
	}
}

func TestCanvasPixels(t *testing.T) {
	cv := NewCanvas(3, 2)
	cv.Pixel(0, 0, skin)     // top half only
	cv.Pixel(1, 1, skin)     // bottom half only
	cv.Pixel(2, 2, skin)     // top and bottom
	cv.Pixel(2, 3, suitBlue) // of the same cell
	cv.Pixel(0, -1, skin)    // out of bounds: ignored
	cv.Pixel(0, 4, skin)
	got := string([]rune{cv.At(0, 0), cv.At(1, 0), cv.At(2, 0), cv.At(2, 1), cv.At(0, 1)})
	if got != "▀▄ ▀ " {
		t.Errorf("cells = %q, want %q", got, "▀▄ ▀ ")
	}
}

func TestSpritesAreRectangular(t *testing.T) {
	sprites := []Sprite{heroStanding, heroCrouching, heroGliding[0], heroGliding[1], swingReach.Sprite, swingHang.Sprite}
	for i, s := range sprites {
		for _, row := range s.rows {
			if len(row) != s.W() {
				t.Errorf("sprite %d: row %q is %d wide, want %d", i, row, len(row), s.W())
			}
			for _, b := range []byte(row) {
				if _, ok := s.palette[b]; !ok && b != '.' {
					t.Errorf("sprite %d: %q is not in the palette", i, b)
				}
			}
		}
	}
	for _, p := range []pose{swingReach, swingHang} {
		if _, ok := p.palette[p.rows[0][p.hand]]; !ok {
			t.Errorf("the swinger's hand (column %d) should be on its top row", p.hand)
		}
	}
}

func TestCanvasRender(t *testing.T) {
	cv := NewCanvas(4, 2)
	cv.Text(0, 0, "ab", nil)
	cv.Set(3, 1, 'z', skin)
	cv.Set(9, 9, 'x', skin) // out of bounds: ignored
	out := cv.Render()
	if got := maxLineWidth(out); got != 4 {
		t.Errorf("width = %d, want 4", got)
	}
	if want := fmt.Sprint("ab  "); !containsPlain(out, want) {
		t.Errorf("render = %q", out)
	}
	if cv.At(3, 1) != 'z' || cv.At(9, 9) != 0 {
		t.Error("At should read back cells and return 0 outside")
	}
}

func containsPlain(styled, plain string) bool {
	var b []rune
	esc := false
	for _, r := range styled {
		switch {
		case r == 0x1b:
			esc = true
		case esc && (r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z'):
			esc = false
		case !esc:
			b = append(b, r)
		}
	}
	return len(b) >= len([]rune(plain)) && string(b[:len([]rune(plain))]) == plain
}
