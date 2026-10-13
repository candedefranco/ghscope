package ui

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/candedefranco/ghscope/internal/github"
)

// Fetcher loads a user. *github.Client satisfies it; tests can pass a fake.
type Fetcher interface {
	FetchUser(ctx context.Context, login string) (*github.User, error)
}

const (
	twoColumnMinWidth = 100
	maxWidth          = 120
	listRows          = 8
	minListRows       = 3
	helpLines         = 2 // a blank line and the key help under the dashboard
)

// Layout is how much room the city and the repository list get, so the whole
// dashboard fits in the terminal's height.
type Layout struct {
	Floors   int  // height of the city's tallest building
	ListRows int  // repositories shown at once
	Compact  bool // no blank lines between panels
}

// FullLayout is the dashboard with no height limit.
var FullLayout = Layout{Floors: maxFloors, ListRows: listRows}

// Options control how the dashboard is drawn.
type Options struct {
	Mode   ViewMode
	Lights float64 // skyline intro progress, 0..1
	Anim   *Anim   // animation drawn over the skyline, if any
	Layout Layout  // zero value means FullLayout
}

// Dashboard lays out the whole screen for a loaded user. It is a pure function
// of its inputs, used both by the interactive view and by --once.
func Dashboard(u *github.User, cursor, width int, now time.Time, opts Options) string {
	if opts.Layout == (Layout{}) {
		opts.Layout = FullLayout
	}
	width = min(width, maxWidth)
	header := Header(u, width)
	cal := u.Contributions.Calendar
	contrib := Panel("contributions", Contributions(cal, Inner(width), opts), width)
	rows := opts.Layout.ListRows
	join := func(panels ...string) string {
		if opts.Layout.Compact {
			return strings.Join(panels, "\n")
		}
		return strings.Join(panels, "\n\n")
	}

	repos := u.Repositories.Nodes
	if width >= twoColumnMinWidth {
		const gap = 2
		left := (width - gap) * 2 / 5
		right := width - gap - left
		langs := Panel("languages", Languages(repos, Inner(left)), left)
		list := Panel("repositories", Repos(repos, cursor, rows, Inner(right), now), right)
		bottom := lipgloss.JoinHorizontal(lipgloss.Top, langs, strings.Repeat(" ", gap), list)
		return join(header, contrib, bottom)
	}
	langs := Panel("languages", Languages(repos, Inner(width)), width)
	list := Panel("repositories", Repos(repos, cursor, rows, Inner(width), now), width)
	return join(header, contrib, langs, list)
}

// FitLayout finds the roomiest layout whose dashboard, plus the key help,
// fits in height rows. It gives up room in order of what's missed least: a
// few rows of the repository list (it scrolls), some floors of the city, the
// blank lines between panels, then the rest. height <= 0 means no limit.
func FitLayout(u *github.User, width, height int, mode ViewMode) Layout {
	l := FullLayout
	if height <= 0 {
		return l
	}
	tall := func() bool {
		d := Dashboard(u, 0, width, time.Time{}, Options{Mode: mode, Lights: 1, Layout: l})
		return lipgloss.Height(d)+helpLines > height
	}
	city := mode == ViewSkyline
	for tall() {
		switch {
		case l.ListRows > minListRows+1:
			l.ListRows--
		case city && l.Floors > minFloors+2:
			l.Floors--
		case !l.Compact:
			l.Compact = true
		case city && l.Floors > minFloors:
			l.Floors--
		case l.ListRows > minListRows:
			l.ListRows--
		default:
			return l // as small as it gets; the terminal cuts off the rest
		}
	}
	return l
}

type loadedMsg struct {
	user *github.User
	err  error
}

// frameMsg advances the lights intro or the running animation by one frame.
// idleMsg means it's time to start the next random animation. Both carry the
// generation they were scheduled in: any change of view bumps the generation,
// so stale timers are ignored instead of stacking up and speeding things up.
type (
	frameMsg struct{ gen int }
	idleMsg  struct{ gen int }
)

const (
	frameEvery     = 50 * time.Millisecond
	lightsDuration = 1600 * time.Millisecond
	lightsStep     = float64(frameEvery) / float64(lightsDuration)
	minIdle        = 3 * time.Second
	maxIdle        = 8 * time.Second
)

func frameTick(gen int) tea.Cmd {
	return tea.Tick(frameEvery, func(time.Time) tea.Msg { return frameMsg{gen} })
}

func (m Model) idleTick() tea.Cmd {
	wait := minIdle + time.Duration(m.rng.Int64N(int64(maxIdle-minIdle)))
	gen := m.gen
	return tea.Tick(wait, func(time.Time) tea.Msg { return idleMsg{gen} })
}

// Model is the interactive dashboard.
type Model struct {
	fetcher Fetcher
	login   string
	user    *github.User
	err     error
	loading bool
	cursor  int
	width   int
	height  int
	layout  Layout // fitted to width and height, see FitLayout
	spin    spinner.Model
	now     func() time.Time
	mode    ViewMode
	lights  float64

	animations bool   // play random animations over the skyline
	anim       *Anim  // the animation playing now, if any
	lastEvent  string // so the same one doesn't play twice in a row
	gen        int    // see frameMsg
	rng        *rand.Rand
}

func New(f Fetcher, login string, mode ViewMode, animations bool) Model {
	return Model{
		mode:       mode,
		animations: animations,
		rng:        rand.New(rand.NewPCG(uint64(time.Now().UnixNano()), 0)),
		fetcher:    f,
		login:      login,
		loading:    true,
		width:      twoColumnMinWidth,
		layout:     FullLayout,
		spin:       spinner.New(spinner.WithSpinner(spinner.MiniDot), spinner.WithStyle(sGreen)),
		now:        time.Now,
	}
}

func (m Model) load() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		u, err := m.fetcher.FetchUser(ctx, m.login)
		return loadedMsg{user: u, err: err}
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.load(), m.spin.Tick)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		if msg.Width != m.width {
			m.anim = nil // the city changes shape under it
		}
		m.width, m.height = msg.Width, msg.Height
		m = m.fitted()
	case loadedMsg:
		m.loading = false
		m.user, m.err = msg.user, msg.err
		m.cursor = 0
		m = m.fitted()
		if m.err == nil && m.mode == ViewSkyline {
			return m.startCity()
		}
		m.lights = 1
	case frameMsg:
		if msg.gen != m.gen {
			return m, nil
		}
		return m.nextFrame()
	case idleMsg:
		if msg.gen != m.gen || m.anim != nil {
			return m, nil
		}
		return m.playEvent()
	case spinner.TickMsg:
		if !m.loading {
			return m, nil
		}
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd
	case tea.KeyPressMsg:
		return m.onKey(msg.String())
	}
	return m, nil
}

func (m Model) onKey(key string) (tea.Model, tea.Cmd) {
	n := 0
	if m.user != nil {
		n = len(m.user.Repositories.Nodes)
	}
	switch key {
	case "q", "ctrl+c", "esc":
		return m, tea.Quit
	case "down", "j":
		if m.cursor < n-1 {
			m.cursor++
		}
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "home", "g":
		m.cursor = 0
	case "end", "G":
		m.cursor = max(0, n-1)
	case "r":
		if !m.loading {
			m.loading = true
			m.err = nil
			return m, tea.Batch(m.load(), m.spin.Tick)
		}
	case "enter", "o":
		if m.user != nil && m.cursor < n {
			openBrowser(m.user.Repositories.Nodes[m.cursor].URL)
		}
	case "p":
		if m.user != nil {
			openBrowser(m.user.URL)
		}
	case "v":
		if m.mode == ViewSkyline {
			m.mode = ViewHeatmap
			m.gen++ // stop the intro and animations
			m.anim = nil
			return m.fitted(), nil
		}
		m.mode = ViewSkyline
		return m.fitted().startCity() // replay the city lighting up
	case "a":
		if m.mode == ViewSkyline && m.user != nil {
			m.gen++ // cancel the pending timer and play one now
			m.lights = 1
			return m.playEvent()
		}
	}
	return m, nil
}

func (m Model) View() tea.View {
	var content string
	switch {
	case m.loading && m.user == nil:
		content = fmt.Sprintf("\n  %s fetching @%s from GitHub…", m.spin.View(), m.login)
	case m.err != nil:
		content = "\n  " + sError.Render("error: ") + ErrorHint(m.err) + "\n\n  " + sMuted.Render("r retry · q quit")
	default:
		content = Dashboard(m.user, m.cursor, m.width, m.now(), Options{Mode: m.mode, Lights: m.lights, Anim: m.anim, Layout: m.layout})
		help := "↑/↓ move · enter open repo · p profile · v city/grid · a animate · r refresh · q quit"
		if m.loading {
			help = m.spin.View() + " refreshing…"
		}
		content += "\n\n" + sMuted.Render(Truncate(help, m.width))
	}
	v := tea.NewView(content)
	v.AltScreen = true
	v.WindowTitle = "ghscope · @" + m.login
	return v
}

// fitted recomputes the layout for the terminal size, view and data. A
// running animation is dropped if the city changes height under it.
func (m Model) fitted() Model {
	if m.user == nil {
		return m
	}
	l := FitLayout(m.user, m.width, m.height, m.mode)
	if l.Floors != m.layout.Floors {
		m.anim = nil
	}
	m.layout = l
	return m
}

// startCity begins the lights-on intro from a dark city.
func (m Model) startCity() (tea.Model, tea.Cmd) {
	m.gen++
	m.lights = 0
	m.anim = nil
	return m, frameTick(m.gen)
}

// nextFrame advances the intro, then the current animation, then waits.
func (m Model) nextFrame() (tea.Model, tea.Cmd) {
	switch {
	case m.lights < 1:
		m.lights = min(1, m.lights+lightsStep)
		if m.lights < 1 {
			return m, frameTick(m.gen)
		}
	case m.anim != nil:
		next := &Anim{Event: m.anim.Event, Frame: m.anim.Frame + 1}
		if next.Frame < next.Event.Frames() {
			m.anim = next
			return m, frameTick(m.gen)
		}
		m.lastEvent = m.anim.Event.Name()
		m.anim = nil
	}
	if !m.animations {
		return m, nil
	}
	return m, m.idleTick()
}

// playEvent starts a random animation that fits the current city.
func (m Model) playEvent() (tea.Model, tea.Cmd) {
	if m.user == nil || m.mode != ViewSkyline {
		return m, nil
	}
	city := NewCity(m.user.Contributions.Calendar, Inner(min(m.width, maxWidth)), m.layout.Floors)
	ev := RandomEvent(city, m.rng, m.lastEvent)
	if ev == nil {
		return m, nil
	}
	m.anim = &Anim{Event: ev}
	return m, frameTick(m.gen)
}

// ErrorHint turns errors into a message that says what to do next.
func ErrorHint(err error) string {
	switch {
	case errors.Is(err, github.ErrNotFound):
		return err.Error() + ". Check the username."
	case errors.Is(err, github.ErrUnauthorized):
		return "GitHub rejected the token. Run `gh auth login` or set a valid GITHUB_TOKEN."
	default:
		return err.Error()
	}
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}
