package ui

import (
	"strings"
	"testing"

	"github.com/candedefranco/ghscope/internal/github"
)

// week builds a week with the given count per day (Sunday first).
func week(start string, counts ...int) github.Week {
	w := github.Week{}
	for i, c := range counts {
		day := start
		if i > 0 {
			day = "" // only the first date matters for labels
		}
		w.Days = append(w.Days, github.Day{Date: day, Count: c})
	}
	return w
}

func TestBuildingsMapWeeksToTheCity(t *testing.T) {
	city := buildings([]github.Week{
		week("2026-01-04", 0, 0, 0, 0, 0, 0, 0),   // nothing: empty lot
		week("2026-01-11", 10, 10, 0, 0, 0, 0, 0), // best week, 2 active days
		week("2026-01-18", 1, 1, 1, 1, 1, 0, 0),   // small but steady: 5 active days
	}, maxFloors)
	if city[0].floors != 0 {
		t.Errorf("an empty week should be an empty lot, got %d floors", city[0].floors)
	}
	if city[1].floors != maxFloors || !city[1].hasAntenna || !city[1].bright {
		t.Errorf("the best week should be the tallest, bright, with the antenna: %+v", city[1])
	}
	if city[2].floors < 1 || city[2].floors >= city[1].floors || city[2].hasAntenna {
		t.Errorf("a small week should be short, without antenna: %+v", city[2])
	}
	if city[2].litShare <= city[1].litShare {
		t.Errorf("more active days should light more windows: %v vs %v", city[2].litShare, city[1].litShare)
	}
}

func TestWindowsLightUpProgressively(t *testing.T) {
	b := building{floors: maxFloors, litShare: 1}
	count := func(lights float64) int {
		n := 0
		for w := range 50 {
			for f := range maxFloors {
				if windowLit(w, f, b, lights) {
					n++
				}
			}
		}
		return n
	}
	if got := count(0); got != 0 {
		t.Errorf("with the lights off, %d windows are lit", got)
	}
	half, all := count(0.5), count(1)
	if all != 50*maxFloors {
		t.Errorf("a fully active week should end with every window lit, got %d", all)
	}
	if half <= all/4 || half >= all*3/4 {
		t.Errorf("halfway through the intro, %d of %d windows are lit", half, all)
	}
	if !windowLit(3, 4, b, 1) || windowLit(3, 4, building{litShare: 0}, 1) {
		t.Error("a week with no active days should have no lit windows")
	}
}

func TestSkylineIsDeterministicAndFits(t *testing.T) {
	cal := fakeUser(0).Contributions.Calendar
	for _, width := range []int{20, 56, 106} {
		a := Skyline(cal, width, maxFloors, 1, nil)
		if a != Skyline(cal, width, maxFloors, 1, nil) {
			t.Fatal("the same data should always draw the same city")
		}
		lines := strings.Split(a, "\n")
		if want := skyRows + maxFloors + 2; len(lines) != want { // + ground + month labels
			t.Errorf("width %d: %d lines, want %d", width, len(lines), want)
		}
		if got := maxLineWidth(a); got > width {
			t.Errorf("width %d: skyline is %d wide", width, got)
		}
	}
}

func TestSkylineShowsTheAntennaAndMoon(t *testing.T) {
	out := Skyline(fakeUser(0).Contributions.Calendar, 106, maxFloors, 1, nil)
	for _, want := range []string{"•", "│", "●", "▔"} {
		if !strings.Contains(out, want) {
			t.Errorf("skyline is missing %q", want)
		}
	}
}

func TestBestWeek(t *testing.T) {
	cal := github.Calendar{Weeks: []github.Week{
		week("2026-03-01", 1, 1),
		week("2026-03-08", 5, 5),
		week("2026-03-15", 2),
	}}
	start, total, ok := BestWeek(cal)
	if !ok || total != 10 || start.Day() != 8 {
		t.Errorf("BestWeek = %v %d %v", start, total, ok)
	}
	if _, _, ok := BestWeek(github.Calendar{}); ok {
		t.Error("an empty calendar has no best week")
	}
}

func TestParseViewMode(t *testing.T) {
	for in, want := range map[string]ViewMode{"skyline": ViewSkyline, "city": ViewSkyline, "heatmap": ViewHeatmap} {
		if got, err := ParseViewMode(in); err != nil || got != want {
			t.Errorf("ParseViewMode(%q) = %v, %v", in, got, err)
		}
	}
	if _, err := ParseViewMode("pie"); err == nil {
		t.Error("unknown views should be rejected")
	}
}
