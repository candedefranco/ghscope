package ui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

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
	m := New(f, "octo", ViewSkyline, true)
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
	m := New(fakeFetcher{user: fakeUser(3)}, "octo", ViewSkyline, true)
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
	m := New(fakeFetcher{user: fakeUser(2)}, "octo", ViewSkyline, true)
	next, cmd := m.Update(m.load()())
	m = next.(Model)
	if m.lights != 0 || cmd == nil {
		t.Fatal("loading the skyline should start the lights intro")
	}
	for range 100 {
		next, cmd = m.Update(frameMsg{m.gen})
		m = next.(Model)
		if m.lights >= 1 {
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
	m := New(fakeFetcher{user: fakeUser(1)}, "octo", ViewHeatmap, true)
	next, _ := m.Update(m.load()())
	if next.(Model).lights != 1 {
		t.Error("the heatmap has no intro, so the lights should be fully on")
	}
}

// runFrames feeds frame ticks until the model stops asking for them.
func runFrames(t *testing.T, m Model, limit int) (Model, tea.Cmd) {
	t.Helper()
	var cmd tea.Cmd
	for range limit {
		var next tea.Model
		next, cmd = m.Update(frameMsg{m.gen})
		m = next.(Model)
		if m.lights >= 1 && m.anim == nil {
			return m, cmd
		}
	}
	t.Fatalf("still animating after %d frames", limit)
	return m, nil
}

func TestModelPlaysAnimationOnDemand(t *testing.T) {
	m := New(fakeFetcher{user: fakeUser(3)}, "octo", ViewSkyline, true)
	m.width = 110
	next, _ := m.Update(m.load()())
	m, _ = runFrames(t, next.(Model), 100) // the lights intro
	next, cmd := m.Update(key("a"))
	m = next.(Model)
	if m.anim == nil || cmd == nil {
		t.Fatal("a should start an animation right away")
	}
	name := m.anim.Event.Name()
	if v := m.View().Content; v == "" {
		t.Error("view should render while animating")
	}
	m, cmd = runFrames(t, m, 300)
	if m.lastEvent != name || cmd == nil {
		t.Errorf("after %s ends, the next one should be scheduled (lastEvent=%q)", name, m.lastEvent)
	}
}

func TestModelFitsTheWindow(t *testing.T) {
	m := loaded(t, fakeFetcher{user: fakeUser(12)})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 32})
	m = next.(Model)
	v := m.View().Content
	if got := lipgloss.Height(v); got > 32 {
		t.Errorf("view is %d rows tall in a 32-row window", got)
	}
	if !strings.Contains(v, "languages") || !strings.Contains(v, "q quit") {
		t.Error("the languages panel and the key help should fit")
	}
	if m.layout.Floors >= maxFloors {
		t.Errorf("the city should be lower in a short window, got %d floors", m.layout.Floors)
	}
	next, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 60})
	if next.(Model).layout != FullLayout {
		t.Error("a tall window should get the full layout back")
	}
}

func TestModelIgnoresStaleTimers(t *testing.T) {
	m := New(fakeFetcher{user: fakeUser(3)}, "octo", ViewSkyline, true)
	next, _ := m.Update(m.load()())
	m = next.(Model)
	stale := m.gen
	m = press(m, "v", "v") // to the grid and back: a new generation
	before := m.lights
	next, cmd := m.Update(frameMsg{stale})
	if next.(Model).lights != before || cmd != nil {
		t.Error("a frame from an old generation must be ignored")
	}
	if _, cmd := m.Update(idleMsg{stale}); cmd != nil {
		t.Error("an idle timer from an old generation must be ignored")
	}
}

func TestModelGridStopsAnimations(t *testing.T) {
	m := New(fakeFetcher{user: fakeUser(3)}, "octo", ViewSkyline, true)
	m.width = 110
	next, _ := m.Update(m.load()())
	m = press(next.(Model), "a")
	if m.anim == nil {
		t.Fatal("expected an animation")
	}
	if m = press(m, "v"); m.anim != nil {
		t.Error("switching to the grid should stop the animation")
	}
	if m = press(m, "a"); m.anim != nil {
		t.Error("a does nothing on the grid")
	}
}

func TestModelWithoutAnimationsStaysStill(t *testing.T) {
	m := New(fakeFetcher{user: fakeUser(3)}, "octo", ViewSkyline, false)
	next, _ := m.Update(m.load()())
	m, cmd := runFrames(t, next.(Model), 100)
	if cmd != nil {
		t.Error("with animations off, nothing should be scheduled after the intro")
	}
	if m.anim != nil {
		t.Error("no animation should be playing")
	}
}
