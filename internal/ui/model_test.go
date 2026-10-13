package ui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/candedefranco/ghscope/internal/github"
)

type fakeFetcher struct {
	user *github.User
	err  error
}

func (f fakeFetcher) FetchUser(context.Context, string) (*github.User, error) { return f.user, f.err }

func key(s string) tea.KeyPressMsg {
	switch s {
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	}
	return tea.KeyPressMsg{Code: rune(s[0]), Text: s}
}

// loaded runs the model's fetch command and feeds the result back in.
func loaded(t *testing.T, f fakeFetcher) Model {
	t.Helper()
	m := New(f, "octo", ViewSkyline)
	m.now = func() time.Time { return now }
	next, _ := m.Update(m.load()())
	return next.(Model)
}

func press(m Model, keys ...string) Model {
	for _, k := range keys {
		next, _ := m.Update(key(k))
		m = next.(Model)
	}
	return m
}

func TestModelShowsLoadingThenDashboard(t *testing.T) {
	m := New(fakeFetcher{user: fakeUser(3)}, "octo", ViewSkyline)
	if v := m.View().Content; !strings.Contains(v, "fetching @octo") {
		t.Errorf("loading view = %q", v)
	}
	m = loaded(t, fakeFetcher{user: fakeUser(3)})
	if v := m.View().Content; !strings.Contains(v, "Octo Cat") || !strings.Contains(v, "q quit") {
		t.Errorf("dashboard view missing content")
	}
	if !m.View().AltScreen {
		t.Error("the dashboard should use the alternate screen")
	}
}

func TestModelCursorStaysInBounds(t *testing.T) {
	m := loaded(t, fakeFetcher{user: fakeUser(3)})
	if m = press(m, "up"); m.cursor != 0 {
		t.Errorf("cursor = %d after up at the top", m.cursor)
	}
	if m = press(m, "down", "down", "down", "down"); m.cursor != 2 {
		t.Errorf("cursor = %d after scrolling past the end, want 2", m.cursor)
	}
	if m = press(m, "k"); m.cursor != 1 {
		t.Errorf("cursor = %d after k, want 1", m.cursor)
	}
	if m = press(m, "G"); m.cursor != 2 {
		t.Errorf("cursor = %d after G, want 2", m.cursor)
	}
	if m = press(m, "g"); m.cursor != 0 {
		t.Errorf("cursor = %d after g, want 0", m.cursor)
	}
}

func TestModelQuits(t *testing.T) {
	m := loaded(t, fakeFetcher{user: fakeUser(1)})
	_, cmd := m.Update(key("q"))
	if cmd == nil {
		t.Fatal("q should return a command")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Error("q should quit")
	}
}

func TestModelShowsHelpfulErrors(t *testing.T) {
	cases := map[error]string{
		github.ErrUnauthorized:                             "gh auth login",
		errors.Join(github.ErrNotFound, errors.New("bob")): "Check the username",
		errors.New("network down"):                         "network down",
	}
	for err, want := range cases {
		m := loaded(t, fakeFetcher{err: err})
		if v := m.View().Content; !strings.Contains(v, want) {
			t.Errorf("error view for %v = %q, want it to mention %q", err, v, want)
		}
	}
}

func TestModelRefreshes(t *testing.T) {
	m := loaded(t, fakeFetcher{user: fakeUser(2)})
	next, cmd := m.Update(key("r"))
	m = next.(Model)
	if !m.loading || cmd == nil {
		t.Error("r should start a new fetch")
	}
	if v := m.View().Content; !strings.Contains(v, "refreshing") || !strings.Contains(v, "Octo Cat") {
		t.Error("while refreshing, the old dashboard should stay visible")
	}
}

func TestModelLightsTheCityAndToggles(t *testing.T) {
	m := New(fakeFetcher{user: fakeUser(2)}, "octo", ViewSkyline)
	next, cmd := m.Update(m.load()())
	m = next.(Model)
	if m.lights != 0 || cmd == nil {
		t.Fatal("loading the skyline should start the lights intro")
	}
	for range 100 {
		next, cmd = m.Update(lightsMsg{})
		m = next.(Model)
		if cmd == nil {
			break
		}
	}
	if m.lights != 1 {
		t.Errorf("lights = %v after the intro, want 1", m.lights)
	}
	if m = press(m, "v"); m.mode != ViewHeatmap {
		t.Error("v should switch to the heatmap")
	}
	next, cmd = m.Update(key("v"))
	m = next.(Model)
	if m.mode != ViewSkyline || m.lights != 0 || cmd == nil {
		t.Error("switching back to the city should replay the lights")
	}
}

func TestModelHeatmapNeedsNoIntro(t *testing.T) {
	m := New(fakeFetcher{user: fakeUser(1)}, "octo", ViewHeatmap)
	next, _ := m.Update(m.load()())
	if next.(Model).lights != 1 {
		t.Error("the heatmap has no intro, so the lights should be fully on")
	}
}
