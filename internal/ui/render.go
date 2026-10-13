// Package ui renders the ghscope dashboard. render.go holds the pure drawing
// functions; model.go wires them into an interactive Bubble Tea program.
package ui

import (
	"fmt"
	"math"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/candedefranco/ghscope/internal/github"
	"github.com/candedefranco/ghscope/internal/stats"
)

// Palette: GitHub dark, shared with the author's profile and portfolio.
var (
	colFg     = lipgloss.Color("#e6edf3")
	colMuted  = lipgloss.Color("#8b949e")
	colLine   = lipgloss.Color("#30363d")
	colGreen  = lipgloss.Color("#7ee787")
	colBlue   = lipgloss.Color("#79c0ff")
	colPurple = lipgloss.Color("#d2a8ff")
	colRed    = lipgloss.Color("#ff7b72")
	colYellow = lipgloss.Color("#e3b341")

	// Contribution levels 0–4, blue → purple.
	heatColors = []string{"#161b22", "#1f3a5f", "#3a62b0", "#7b83e8", "#d2a8ff"}
)

var (
	sMuted  = lipgloss.NewStyle().Foreground(colMuted)
	sBold   = lipgloss.NewStyle().Foreground(colFg).Bold(true)
	sGreen  = lipgloss.NewStyle().Foreground(colGreen).Bold(true)
	sBlue   = lipgloss.NewStyle().Foreground(colBlue).Bold(true)
	sPurple = lipgloss.NewStyle().Foreground(colPurple)
	sYellow = lipgloss.NewStyle().Foreground(colYellow)
	sError  = lipgloss.NewStyle().Foreground(colRed).Bold(true)
)

// level maps a day's count to 0–4 relative to the busiest day, like GitHub's graph.
func level(count, max int) int {
	if count <= 0 || max <= 0 {
		return 0
	}
	return min(4, 1+int(math.Floor(float64(count-1)/float64(max)*4)))
}

var monthNames = [...]string{"Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"}

// monthLabels returns a row with each month's name above the week where it
// starts, skipping a label when it would overlap the previous one.
func monthLabels(weeks []github.Week, cellW int) string {
	row := []rune(strings.Repeat(" ", len(weeks)*cellW))
	lastMonth, lastEnd := -1, -1
	for i, w := range weeks {
		if len(w.Days) == 0 {
			continue
		}
		t, err := time.Parse("2006-01-02", w.Days[0].Date)
		if err != nil {
			continue
		}
		m := int(t.Month()) - 1
		pos := i * cellW
		if m != lastMonth && pos > lastEnd && pos+3 <= len(row) {
			copy(row[pos:], []rune(monthNames[m]))
			lastEnd = pos + 3
		}
		lastMonth = m
	}
	return string(row)
}

// minSpacedWeeks: below this many weeks, drop the gaps between cells to show more history.
const minSpacedWeeks = 40

// Heatmap draws the contribution calendar: one column per week, one row per
// weekday, plus month labels. It keeps the most recent weeks that fit in width,
// spacing cells like GitHub does unless that would hide too much history.
func Heatmap(cal github.Calendar, width int) string {
	const labelW = 4 // "Mon "
	weeks := cal.Weeks
	cell := "■ "
	if spaced := (width - labelW) / 2; spaced < len(weeks) && spaced < minSpacedWeeks {
		cell = "■"
	}
	cw := lipgloss.Width(cell)
	if fit := (width - labelW) / cw; fit < len(weeks) {
		weeks = weeks[max(0, len(weeks)-fit):]
	}

	maxCount := 0
	for _, w := range weeks {
		for _, d := range w.Days {
			maxCount = max(maxCount, d.Count)
		}
	}

	var b strings.Builder
	b.WriteString(strings.Repeat(" ", labelW) + sMuted.Render(strings.TrimRight(monthLabels(weeks, cw), " ")) + "\n")
	dayLabels := []string{"", "Mon", "", "Wed", "", "Fri", ""}
	for wd := range 7 {
		b.WriteString(sMuted.Render(fmt.Sprintf("%-*s", labelW, dayLabels[wd])))
		for i, w := range weeks {
			// The first week may start mid-week; GitHub's weeks begin on Sunday.
			offset := 7 - len(w.Days)
			if i != 0 {
				offset = 0
			}
			di := wd - offset
			if di < 0 || di >= len(w.Days) {
				b.WriteString(strings.Repeat(" ", cw))
				continue
			}
			c := heatColors[level(w.Days[di].Count, maxCount)]
			b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(c)).Render(cell))
		}
		if wd < 6 {
			b.WriteString("\n")
		}
	}
	return b.String()
}

// Bar draws a horizontal bar filled to share (0..1) of width cells.
func Bar(share float64, width int, color string) string {
	filled := int(math.Round(share * float64(width)))
	filled = max(0, min(width, filled))
	if share > 0 && filled == 0 {
		filled = 1 // tiny shares still show up
	}
	c := lipgloss.Color(color)
	if color == "" {
		c = colMuted
	}
	return lipgloss.NewStyle().Foreground(c).Render(strings.Repeat("█", filled)) +
		lipgloss.NewStyle().Foreground(colLine).Render(strings.Repeat("░", width-filled))
}

// TimeAgo formats how long ago t was, compactly ("3d ago").
func TimeAgo(t, now time.Time) string {
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	case d < 365*24*time.Hour:
		return fmt.Sprintf("%dmo ago", int(d.Hours()/24/30))
	default:
		return fmt.Sprintf("%dy ago", int(d.Hours()/24/365))
	}
}

// Truncate shortens s to at most n cells, adding an ellipsis.
func Truncate(s string, n int) string {
	if n <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n == 1 {
		return "…"
	}
	return string(r[:n-1]) + "…"
}

// Header shows who the user is and headline numbers.
func Header(u *github.User, width int) string {
	name := sBold.Render(u.Name)
	if u.Name == "" {
		name = sBold.Render(u.Login)
	}
	title := sGreen.Render("ghscope") + sMuted.Render(" · ") + name + sMuted.Render(" @"+u.Login)
	nums := sMuted.Render(fmt.Sprintf("%d followers · ★ %d · %d public repos",
		u.Followers.TotalCount, stats.TotalStars(u.Repositories.Nodes), u.Repositories.TotalCount))
	// Numbers go on the right, or on their own line when the terminal is narrow
	line := title + "\n" + nums
	if gap := width - lipgloss.Width(title) - lipgloss.Width(nums); gap >= 2 {
		line = title + strings.Repeat(" ", gap) + nums
	}
	var about []string
	if u.Bio != "" {
		about = append(about, Truncate(strings.ReplaceAll(u.Bio, "\n", " "), width-4))
	}
	if u.Location != "" {
		about = append(about, "📍 "+u.Location)
	}
	if len(about) == 0 {
		return line
	}
	return line + "\n" + sMuted.Render(strings.Join(about, "  "))
}

// panelChrome is the horizontal space a Panel's border and padding take.
const panelChrome = 4

// Inner is the content width available inside a Panel of the given outer width.
func Inner(outer int) int { return outer - panelChrome }

// Panel wraps content in a rounded box with a title. outer is the total width,
// border included (Lip Gloss v2 widths include borders and padding); content
// must fit in Inner(outer).
func Panel(title, content string, outer int) string {
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colLine).
		Padding(0, 1).
		Width(outer)
	return sBlue.Render(" "+title) + "\n" + box.Render(content)
}

// ViewMode picks how contributions are drawn.
type ViewMode int

const (
	ViewSkyline ViewMode = iota // the city at night (default)
	ViewHeatmap                 // GitHub's classic grid
)

// ParseViewMode reads a --view flag value.
func ParseViewMode(s string) (ViewMode, error) {
	switch s {
	case "skyline", "city":
		return ViewSkyline, nil
	case "heatmap", "grid":
		return ViewHeatmap, nil
	}
	return 0, fmt.Errorf("unknown view %q (use skyline or heatmap)", s)
}

// Contributions is the skyline or heatmap with its summary line. lights is the
// skyline intro's progress (0..1).
func Contributions(cal github.Calendar, inner int, mode ViewMode, lights float64, anim *Anim) string {
	days := stats.Days(cal)
	s := stats.ComputeStreaks(days)
	unit := func(n int) string {
		if n == 1 {
			return "day"
		}
		return "days"
	}
	parts := []string{
		sBold.Render(fmt.Sprint(cal.Total)) + " contributions in the last year",
		"current streak " + sBold.Render(fmt.Sprint(s.Current)) + " " + unit(s.Current),
		"longest " + sBold.Render(fmt.Sprint(s.Longest)) + " " + unit(s.Longest),
		sBold.Render(fmt.Sprint(stats.ActiveDays(days))) + " active days",
	}
	graph := Heatmap(cal, inner)
	if mode == ViewSkyline {
		graph = Skyline(cal, inner, lights, anim)
		if best := bestWeekLabel(cal); best != "" {
			parts = append(parts, best)
		}
	}
	return graph + "\n\n" + sMuted.Render(JoinWrapped(parts, " · ", inner))
}

// JoinWrapped joins parts with sep, starting a new line instead of splitting
// a part when the next one wouldn't fit in width.
func JoinWrapped(parts []string, sep string, width int) string {
	var lines []string
	line := ""
	for _, p := range parts {
		switch {
		case line == "":
			line = p
		case lipgloss.Width(line+sep+p) <= width:
			line += sep + p
		default:
			lines = append(lines, line)
			line = p
		}
	}
	if line != "" {
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

// Languages lists the top languages with bars.
func Languages(repos []github.Repo, inner int) string {
	langs := stats.TopLanguages(repos, 6)
	if len(langs) == 0 {
		return sMuted.Render("no code in public repositories yet")
	}
	const nameW, pctW = 12, 5
	barW := max(4, inner-nameW-pctW-2)
	var rows []string
	for _, l := range langs {
		rows = append(rows, fmt.Sprintf("%-*s %s %*s",
			nameW, Truncate(l.Name, nameW), Bar(l.Share, barW, l.Color), pctW, fmt.Sprintf("%.0f%%", l.Share*100)))
	}
	return strings.Join(rows, "\n")
}

// Repos lists repositories with the selected one highlighted and described.
// Only `visible` rows are shown, scrolled to keep the cursor in view.
func Repos(repos []github.Repo, cursor, visible, inner int, now time.Time) string {
	if len(repos) == 0 {
		return sMuted.Render("no public repositories")
	}
	start := max(0, min(cursor-visible/2, len(repos)-visible))
	end := min(len(repos), start+visible)
	var rows []string
	for i := start; i < end; i++ {
		r := repos[i]
		lang := ""
		if r.PrimaryLanguage != nil {
			lang = r.PrimaryLanguage.Name
		}
		meta := fmt.Sprintf("★%d  %-10s %8s", r.Stars, Truncate(lang, 10), TimeAgo(r.PushedAt, now))
		nameW := max(8, inner-lipgloss.Width(meta)-3)
		name := fmt.Sprintf("%-*s", nameW, Truncate(r.Name, nameW))
		if i == cursor {
			rows = append(rows, sGreen.Render("› "+name)+" "+sPurple.Render(meta))
		} else {
			rows = append(rows, "  "+name+" "+sMuted.Render(meta))
		}
	}
	desc := "no description"
	if c := repos[cursor]; c.Description != "" {
		desc = c.Description
	}
	more := ""
	if len(repos) > visible {
		more = sMuted.Render(fmt.Sprintf("  %d/%d", cursor+1, len(repos)))
	}
	return strings.Join(rows, "\n") + "\n\n" + sYellow.Render(Truncate(desc, inner)) + more
}
