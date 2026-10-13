package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/candedefranco/ghscope/internal/github"
)

var now = time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)

// fakeUser builds a year of contributions and a few repositories.
func fakeUser(repoCount int) *github.User {
	u := &github.User{Login: "octo", Name: "Octo Cat", Bio: "builds things", Location: "Buenos Aires"}
	u.Followers.TotalCount = 3
	start := now.AddDate(0, 0, -364)
	start = start.AddDate(0, 0, -int(start.Weekday())) // weeks start on Sunday
	var week github.Week
	for d := start; !d.After(now); d = d.AddDate(0, 0, 1) {
		week.Days = append(week.Days, github.Day{Date: d.Format("2006-01-02"), Count: (d.YearDay() * 7) % 5})
		if d.Weekday() == time.Saturday {
			u.Contributions.Calendar.Weeks = append(u.Contributions.Calendar.Weeks, week)
			week = github.Week{}
		}
	}
	if len(week.Days) > 0 {
		u.Contributions.Calendar.Weeks = append(u.Contributions.Calendar.Weeks, week)
	}
	u.Contributions.Calendar.Total = 500
	for i := range repoCount {
		r := github.Repo{
			Name:            fmt.Sprintf("a-repository-with-a-rather-long-name-%d", i),
			Description:     strings.Repeat("A long description. ", 10),
			Stars:           i,
			PushedAt:        now.Add(-time.Duration(i) * 24 * time.Hour),
			PrimaryLanguage: &github.Language{Name: "TypeScript", Color: "#3178c6"},
		}
		r.Languages.Edges = []github.LanguageEdge{
			{Size: int64(1000 * (i + 1)), Node: github.Language{Name: "TypeScript", Color: "#3178c6"}},
			{Size: 300, Node: github.Language{Name: fmt.Sprintf("Lang%d", i)}},
		}
		u.Repositories.Nodes = append(u.Repositories.Nodes, r)
	}
	u.Repositories.TotalCount = repoCount
	return u
}

func maxLineWidth(s string) int {
	w := 0
	for _, line := range strings.Split(s, "\n") {
		w = max(w, lipgloss.Width(line))
	}
	return w
}

func TestDashboardFitsTheTerminal(t *testing.T) {
	u := fakeUser(20)
	for _, width := range []int{60, 80, 99, 100, 110, 120, 200} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			got := maxLineWidth(Dashboard(u, 5, width, now, Options{Lights: 1}))
			if limit := min(width, maxWidth); got > limit {
				t.Errorf("widest line is %d cells, terminal is %d", got, limit)
			}
		})
	}
}

func TestDashboardFitsTheTerminalHeight(t *testing.T) {
	u := fakeUser(12)
	full := lipgloss.Height(Dashboard(u, 0, 120, now, Options{Lights: 1})) + helpLines
	if l := FitLayout(u, 120, 0, ViewSkyline); l != FullLayout {
		t.Errorf("with no height limit the layout should be full, got %+v", l)
	}
	if l := FitLayout(u, 120, full, ViewSkyline); l != FullLayout {
		t.Errorf("with room to spare the layout should be full, got %+v", l)
	}
	for _, mode := range []ViewMode{ViewSkyline, ViewHeatmap} {
		for height := 30; height < full; height++ {
			l := FitLayout(u, 120, height, mode)
			out := Dashboard(u, 0, 120, now, Options{Mode: mode, Lights: 1, Layout: l})
			if got := lipgloss.Height(out) + helpLines; got > height {
				t.Errorf("mode %v, height %d: dashboard needs %d rows (%+v)", mode, height, got, l)
			}
			for _, want := range []string{"contributions", "languages", "repositories"} {
				if !strings.Contains(out, want) {
					t.Errorf("mode %v, height %d: %q is missing", mode, height, want)
				}
			}
		}
	}
	// The city gives up floors before the dashboard drops its spacing, and
	// never goes below minFloors.
	if l := FitLayout(u, 120, full-6, ViewSkyline); l.Compact || l.Floors == maxFloors {
		t.Errorf("a little short: expected a lower city with spacing kept, got %+v", l)
	}
	if l := FitLayout(u, 120, 5, ViewSkyline); l.Floors != minFloors || l.ListRows != minListRows || !l.Compact {
		t.Errorf("a tiny terminal should get the smallest layout, got %+v", l)
	}
}

func TestDashboardShowsTheEssentials(t *testing.T) {
	out := Dashboard(fakeUser(3), 1, 110, now, Options{Mode: ViewHeatmap})
	for _, want := range []string{"Octo Cat", "@octo", "500", "contributions", "languages", "repositories", "› a-repository"} {
		if !strings.Contains(out, want) {
			t.Errorf("dashboard is missing %q", want)
		}
	}
}

func TestDashboardHandlesEmptyUsers(t *testing.T) {
	out := Dashboard(&github.User{Login: "new"}, 0, 100, now, Options{Lights: 1})
	for _, want := range []string{"@new", "no public repositories", "no code in public repositories"} {
		if !strings.Contains(out, want) {
			t.Errorf("dashboard is missing %q", want)
		}
	}
}

func TestHeatmapKeepsSevenRowsAndFits(t *testing.T) {
	cal := fakeUser(0).Contributions.Calendar
	for _, width := range []int{30, 60, 110} {
		out := Heatmap(cal, width)
		if rows := strings.Count(out, "\n"); rows != 7 { // month row + 7 day rows
			t.Errorf("width %d: %d line breaks, want 7", width, rows)
		}
		if got := maxLineWidth(out); got > width {
			t.Errorf("width %d: heatmap is %d wide", width, got)
		}
	}
}

func TestLevel(t *testing.T) {
	cases := []struct{ count, max, want int }{
		{0, 10, 0}, {1, 10, 1}, {3, 10, 1}, {4, 10, 2}, {10, 10, 4}, {5, 0, 0}, {1, 1, 1},
	}
	for _, c := range cases {
		if got := level(c.count, c.max); got != c.want {
			t.Errorf("level(%d, %d) = %d, want %d", c.count, c.max, got, c.want)
		}
	}
}

func TestBar(t *testing.T) {
	for _, share := range []float64{0, 0.001, 0.5, 1, 1.2} {
		if got := lipgloss.Width(Bar(share, 20, "#ffffff")); got != 20 {
			t.Errorf("Bar(%v) is %d wide, want 20", share, got)
		}
	}
	if !strings.Contains(Bar(0.001, 20, ""), "█") {
		t.Error("a tiny share should still show one block")
	}
}

func TestTimeAgo(t *testing.T) {
	cases := map[time.Duration]string{
		10 * time.Second:     "just now",
		5 * time.Minute:      "5m ago",
		3 * time.Hour:        "3h ago",
		4 * 24 * time.Hour:   "4d ago",
		65 * 24 * time.Hour:  "2mo ago",
		800 * 24 * time.Hour: "2y ago",
	}
	for d, want := range cases {
		if got := TimeAgo(now.Add(-d), now); got != want {
			t.Errorf("TimeAgo(-%v) = %q, want %q", d, got, want)
		}
	}
}

func TestTruncate(t *testing.T) {
	cases := []struct {
		in   string
		n    int
		want string
	}{
		{"hello", 10, "hello"}, {"hello", 5, "hello"}, {"hello", 4, "hel…"}, {"hello", 1, "…"}, {"hello", 0, ""},
		{"señales", 4, "señ…"}, // runes, not bytes
	}
	for _, c := range cases {
		if got := Truncate(c.in, c.n); got != c.want {
			t.Errorf("Truncate(%q, %d) = %q, want %q", c.in, c.n, got, c.want)
		}
	}
}

func TestJoinWrapped(t *testing.T) {
	got := JoinWrapped([]string{"aaa", "bbb", "ccc"}, " · ", 9)
	if got != "aaa · bbb\nccc" {
		t.Errorf("got %q", got)
	}
}
