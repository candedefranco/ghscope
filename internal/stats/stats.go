// Package stats derives summary numbers from a GitHub user's data.
package stats

import (
	"sort"

	"github.com/candedefranco/ghscope/internal/github"
)

// Days flattens a contribution calendar into chronological order.
func Days(cal github.Calendar) []github.Day {
	var days []github.Day
	for _, w := range cal.Weeks {
		days = append(days, w.Days...)
	}
	return days
}

type Streaks struct {
	Current int // consecutive days with contributions, ending today (or yesterday)
	Longest int
}

// ComputeStreaks counts consecutive days with contributions. A quiet "today"
// doesn't break the current streak, since the day isn't over yet.
func ComputeStreaks(days []github.Day) Streaks {
	var s Streaks
	run := 0
	for _, d := range days {
		if d.Count > 0 {
			run++
			s.Longest = max(s.Longest, run)
		} else {
			run = 0
		}
	}
	for i := len(days) - 1; i >= 0; i-- {
		if days[i].Count > 0 {
			s.Current++
		} else if i != len(days)-1 {
			break
		}
	}
	return s
}

// ActiveDays counts days with at least one contribution.
func ActiveDays(days []github.Day) int {
	n := 0
	for _, d := range days {
		if d.Count > 0 {
			n++
		}
	}
	return n
}

// TotalStars sums stars across the given repositories.
func TotalStars(repos []github.Repo) int {
	total := 0
	for _, r := range repos {
		total += r.Stars
	}
	return total
}

type LanguageShare struct {
	Name  string
	Color string
	Bytes int64
	Share float64 // 0..1
}

// TopLanguages sums code size per language across repositories and returns
// the n largest, with any remainder grouped as "Other".
func TopLanguages(repos []github.Repo, n int) []LanguageShare {
	sizes := map[string]*LanguageShare{}
	var total int64
	for _, r := range repos {
		for _, e := range r.Languages.Edges {
			l, ok := sizes[e.Node.Name]
			if !ok {
				l = &LanguageShare{Name: e.Node.Name, Color: e.Node.Color}
				sizes[e.Node.Name] = l
			}
			l.Bytes += e.Size
			total += e.Size
		}
	}
	if total == 0 {
		return nil
	}
	all := make([]LanguageShare, 0, len(sizes))
	for _, l := range sizes {
		all = append(all, *l)
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].Bytes != all[j].Bytes {
			return all[i].Bytes > all[j].Bytes
		}
		return all[i].Name < all[j].Name
	})
	if len(all) > n {
		other := LanguageShare{Name: "Other"}
		for _, l := range all[n:] {
			other.Bytes += l.Bytes
		}
		all = append(all[:n], other)
	}
	for i := range all {
		all[i].Share = float64(all[i].Bytes) / float64(total)
	}
	return all
}
