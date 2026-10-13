package ui

import (
	"fmt"
	"math"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/candedefranco/ghscope/internal/github"
)

// The skyline draws a year of contributions as a city at night:
//
//   - each building is one week,
//   - its height is how much you contributed that week,
//   - its lit windows are how many days that week you were active,
//   - your best week gets an antenna with a red light.
const (
	maxFloors   = 10
	skyRows     = 3 // room above the tallest building for its antenna and the moon
	buildingW   = 2 // one window plus a strip of wall
	starDensity = 0.035
)

var (
	wallColors    = []string{"#1b2230", "#161c28"} // alternate so neighbouring buildings read apart
	windowOff     = lipgloss.Color("#2d333b")
	windowOn      = lipgloss.Color("#e3b341")
	windowBright  = lipgloss.Color("#ffdf8a")
	antennaColor  = lipgloss.Color("#6e7681")
	antennaLight  = lipgloss.Color("#ff7b72")
	moonColor     = lipgloss.Color("#f0e6c8")
	starColor     = lipgloss.Color("#6e7681")
	groundColor   = lipgloss.Color("#30363d")
	brightTopWeek = 0.75 // weeks above this share of the best week glow brighter
)

// hash01 maps integers to a stable pseudo-random number in [0, 1), so the
// city looks the same on every run.
func hash01(a, b, salt int) float64 {
	h := uint32(a)*374761393 + uint32(b)*668265263 + uint32(salt)*2246822519
	h = (h ^ (h >> 13)) * 1274126177
	h ^= h >> 16
	return float64(h) / float64(math.MaxUint32+1)
}

type building struct {
	floors     int
	litShare   float64 // share of windows that end up lit: active days / 7
	bright     bool
	hasAntenna bool
}

func weekTotals(w github.Week) (total, active int) {
	for _, d := range w.Days {
		total += d.Count
		if d.Count > 0 {
			active++
		}
	}
	return total, active
}

func buildings(weeks []github.Week) []building {
	maxTotal, tallest := 0, -1
	for i, w := range weeks {
		if t, _ := weekTotals(w); t > maxTotal {
			maxTotal, tallest = t, i
		}
	}
	out := make([]building, len(weeks))
	for i, w := range weeks {
		total, active := weekTotals(w)
		if total == 0 || maxTotal == 0 {
			continue // an empty lot
		}
		share := float64(total) / float64(maxTotal)
		out[i] = building{
			floors:     max(1, int(math.Ceil(share*maxFloors))),
			litShare:   float64(active) / 7,
			bright:     share >= brightTopWeek,
			hasAntenna: i == tallest,
		}
	}
	return out
}

// windowLit decides if a window is on. Each window has a fixed chance of being
// lit (its week's active-day share) and a fixed moment it switches on during
// the intro, so `lights` (0..1) reveals the final city gradually.
func windowLit(week, floor int, b building, lights float64) bool {
	return hash01(week, floor, 1) < b.litShare && hash01(week, floor, 2) <= lights
}

// City is the skyline's geometry: which weeks are shown and how tall each
// building is. Animations use it to find roofs to stand on and sky to fly in.
type City struct {
	Weeks     []github.Week
	Buildings []building
	Rows      int // sky + buildings, excluding the ground line
	Cols      int
}

// NewCity lays out the most recent weeks that fit in width.
func NewCity(cal github.Calendar, width int) City {
	weeks := cal.Weeks
	if fit := width / buildingW; fit < len(weeks) {
		weeks = weeks[max(0, len(weeks)-fit):]
	}
	return City{Weeks: weeks, Buildings: buildings(weeks), Rows: skyRows + maxFloors, Cols: len(weeks) * buildingW}
}

// Roof returns the row of the top floor at column x, or Rows for an empty lot.
func (c City) Roof(x int) int {
	i := x / buildingW
	if x < 0 || i >= len(c.Buildings) {
		return c.Rows
	}
	return c.Rows - c.Buildings[i].floors
}

// BuildingX returns the window column of building i.
func (c City) BuildingX(i int) int { return i * buildingW }

// drawCity paints the buildings, antenna, moon and stars onto the canvas.
func drawCity(cv *Canvas, city City, lights float64) {
	moonCol := len(city.Buildings) * 4 / 5
	for i, bd := range city.Buildings {
		x := city.BuildingX(i)
		top := city.Rows - bd.floors
		for r := range city.Rows {
			switch {
			case r >= top && bd.floors > 0:
				wall := lipgloss.Color(wallColors[i%2])
				win := windowOff
				if windowLit(i, city.Rows-1-r, bd, lights) {
					win = windowOn
					if bd.bright {
						win = windowBright
					}
				}
				cv.SetCell(x, r, '▪', win, wall)
				cv.SetCell(x+1, r, ' ', nil, wall)
			case bd.hasAntenna && r == top-1:
				cv.Set(x, r, '│', antennaColor)
			case bd.hasAntenna && r == top-2:
				cv.Set(x, r, '•', antennaLight)
			case r == 0 && i == moonCol:
				cv.Set(x, r, '●', moonColor)
			case hash01(i, r, 3) < starDensity*float64(city.Rows-r)/float64(city.Rows)*2:
				// sky: a few stars, denser higher up
				cv.Set(x, r, '·', starColor)
			}
		}
	}
}

// Anim is an animation in progress over the skyline.
type Anim struct {
	Event Event
	Frame int
}

// Skyline renders the most recent weeks that fit in width as a city at night,
// with month labels underneath. lights (0..1) is how far the "lights turning
// on" intro has progressed; 1 shows the finished city. anim, if not nil, is
// drawn on top.
func Skyline(cal github.Calendar, width int, lights float64, anim *Anim) string {
	city := NewCity(cal, width)
	cv := NewCanvas(city.Cols, city.Rows)
	drawCity(cv, city, lights)
	if anim != nil && anim.Event != nil {
		anim.Event.Draw(cv, city, anim.Frame)
	}
	var b strings.Builder
	b.WriteString(cv.Render() + "\n")
	b.WriteString(lipgloss.NewStyle().Foreground(groundColor).Render(strings.Repeat("▔", city.Cols)))
	b.WriteString("\n" + sMuted.Render(strings.TrimRight(monthLabels(city.Weeks, buildingW), " ")))
	return b.String()
}

// BestWeek returns the week with the most contributions and its total.
func BestWeek(cal github.Calendar) (start time.Time, total int, ok bool) {
	for _, w := range cal.Weeks {
		t, _ := weekTotals(w)
		if t <= total || len(w.Days) == 0 {
			continue
		}
		d, err := time.Parse("2006-01-02", w.Days[0].Date)
		if err != nil {
			continue
		}
		start, total, ok = d, t, true
	}
	return start, total, ok
}

func bestWeekLabel(cal github.Calendar) string {
	start, total, ok := BestWeek(cal)
	if !ok {
		return ""
	}
	return fmt.Sprintf("tallest building: week of %s %d (%s)",
		monthNames[start.Month()-1], start.Day(), sBold.Render(fmt.Sprint(total)))
}
