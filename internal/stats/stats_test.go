package stats

import (
	"math"
	"testing"

	"github.com/candedefranco/ghscope/internal/github"
)

func days(counts ...int) []github.Day {
	out := make([]github.Day, len(counts))
	for i, c := range counts {
		out[i] = github.Day{Count: c}
	}
	return out
}

func TestComputeStreaks(t *testing.T) {
	cases := []struct {
		name             string
		counts           []int
		current, longest int
	}{
		{"empty", nil, 0, 0},
		{"no activity", []int{0, 0, 0}, 0, 0},
		{"active today", []int{0, 1, 2, 3}, 3, 3},
		{"quiet today keeps the streak", []int{1, 1, 0}, 2, 2},
		{"broken yesterday", []int{1, 1, 0, 0}, 0, 2},
		{"longest in the past", []int{1, 1, 1, 1, 0, 1, 1}, 2, 4},
		{"only today", []int{0, 0, 5}, 1, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ComputeStreaks(days(tc.counts...))
			if got.Current != tc.current || got.Longest != tc.longest {
				t.Errorf("got current=%d longest=%d, want %d/%d", got.Current, got.Longest, tc.current, tc.longest)
			}
		})
	}
}

func TestDaysFlattensWeeksInOrder(t *testing.T) {
	cal := github.Calendar{Weeks: []github.Week{
		{Days: []github.Day{{Date: "a"}, {Date: "b"}}},
		{Days: []github.Day{{Date: "c"}}},
	}}
	got := Days(cal)
	if len(got) != 3 || got[0].Date != "a" || got[2].Date != "c" {
		t.Errorf("Days = %+v", got)
	}
}

func TestActiveDaysAndStars(t *testing.T) {
	if got := ActiveDays(days(0, 3, 0, 1)); got != 2 {
		t.Errorf("ActiveDays = %d, want 2", got)
	}
	if got := TotalStars([]github.Repo{{Stars: 2}, {Stars: 5}}); got != 7 {
		t.Errorf("TotalStars = %d, want 7", got)
	}
}

func repo(langs map[string]int64) github.Repo {
	var r github.Repo
	for name, size := range langs {
		r.Languages.Edges = append(r.Languages.Edges, github.LanguageEdge{Size: size, Node: github.Language{Name: name}})
	}
	return r
}

func TestTopLanguages(t *testing.T) {
	repos := []github.Repo{
		repo(map[string]int64{"Go": 600, "C": 100}),
		repo(map[string]int64{"Go": 200, "TypeScript": 50, "CSS": 50}),
	}
	got := TopLanguages(repos, 2)
	want := []struct {
		name  string
		bytes int64
	}{{"Go", 800}, {"C", 100}, {"Other", 100}}
	if len(got) != len(want) {
		t.Fatalf("got %d languages: %+v", len(got), got)
	}
	var sum float64
	for i, w := range want {
		if got[i].Name != w.name || got[i].Bytes != w.bytes {
			t.Errorf("[%d] = %s %d, want %s %d", i, got[i].Name, got[i].Bytes, w.name, w.bytes)
		}
		sum += got[i].Share
	}
	if math.Abs(sum-1) > 1e-9 {
		t.Errorf("shares sum to %v, want 1", sum)
	}
	if math.Abs(got[0].Share-0.8) > 1e-9 {
		t.Errorf("Go share = %v, want 0.8", got[0].Share)
	}
}

func TestTopLanguagesWithoutCode(t *testing.T) {
	if got := TopLanguages([]github.Repo{{}}, 5); got != nil {
		t.Errorf("want nil, got %+v", got)
	}
}
