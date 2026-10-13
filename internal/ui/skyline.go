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

// Skyline renders the most recent weeks that fit in width as a city at night,
// with month labels underneath. lights (0..1) is how far the "lights turning
// on" intro has progressed; 1 shows the finished city.
func Skyline(cal github.Calendar, width int, lights float64) string {
	weeks := cal.Weeks
	if fit := width / buildingW; fit < len(weeks) {
		weeks = weeks[max(0, len(weeks)-fit):]
	}
	city := buildings(weeks)
	rows := skyRows + maxFloors
	moonCol := len(weeks) * 4 / 5

	var b strings.Builder
	for r := range rows {
		for i, bd := range city {
			top := rows - bd.floors // first row of this building
			switch {
			case r >= top && bd.floors > 0:
				floor := rows - 1 - r
				wall := lipgloss.Color(wallColors[i%2])
				win := windowOff
				if windowLit(i, floor, bd, lights) {
					win = windowOn
					if bd.bright {
						win = windowBright
					}
				}
				b.WriteString(lipgloss.NewStyle().Foreground(win).Background(wall).Render("▪") +
					lipgloss.NewStyle().Background(wall).Render(" "))
			case bd.hasAntenna && r == top-1:
				b.WriteString(lipgloss.NewStyle().Foreground(antennaColor).Render("│ "))
			case bd.hasAntenna && r == top-2:
				b.WriteString(lipgloss.NewStyle().Foreground(antennaLight).Render("•") + " ")
			case r == 0 && i == moonCol:
				b.WriteString(lipgloss.NewStyle().Foreground(moonColor).Render("●") + " ")
			default:
				// sky: a few stars, denser higher up
				if hash01(i, r, 3) < starDensity*float64(rows-r)/float64(rows)*2 {
					b.WriteString(lipgloss.NewStyle().Foreground(starColor).Render("·") + " ")
				} else {
					b.WriteString(strings.Repeat(" ", buildingW))
				}
			}
		}
		b.WriteString("\n")
	}
	b.WriteString(lipgloss.NewStyle().Foreground(groundColor).Render(strings.Repeat("▔", len(weeks)*buildingW)))
	b.WriteString("\n" + sMuted.Render(strings.TrimRight(monthLabels(weeks, buildingW), " ")))
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
