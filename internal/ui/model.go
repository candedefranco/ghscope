package ui

import (
	"context"
	"errors"
	"fmt"
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
)

// Options control how the dashboard is drawn.
type Options struct {
	Mode   ViewMode
	Lights float64 // skyline intro progress, 0..1
}

// Dashboard lays out the whole screen for a loaded user. It is a pure function
// of its inputs, used both by the interactive view and by --once.
func Dashboard(u *github.User, cursor, width int, now time.Time, opts Options) string {
	width = min(width, maxWidth)
	header := Header(u, width)
	cal := u.Contributions.Calendar
	contrib := Panel("contributions", Contributions(cal, Inner(width), opts.Mode, opts.Lights), width)

	repos := u.Repositories.Nodes
	if width >= twoColumnMinWidth {
		const gap = 2
		left := (width - gap) * 2 / 5
		right := width - gap - left
		langs := Panel("languages", Languages(repos, Inner(left)), left)
		list := Panel("repositories", Repos(repos, cursor, listRows, Inner(right), now), right)
		bottom := lipgloss.JoinHorizontal(lipgloss.Top, langs, strings.Repeat(" ", gap), list)
		return strings.Join([]string{header, "", contrib, "", bottom}, "\n")
	}
	langs := Panel("languages", Languages(repos, Inner(width)), width)
	list := Panel("repositories", Repos(repos, cursor, listRows, Inner(width), now), width)
	return strings.Join([]string{header, "", contrib, "", langs, "", list}, "\n")
}

type loadedMsg struct {
	user *github.User
	err  error
}

// lightsMsg advances the skyline's "lights turning on" intro.
type lightsMsg struct{}

const (
	lightsDuration = 1600 * time.Millisecond
	lightsFrame    = 40 * time.Millisecond
	lightsStep     = float64(lightsFrame) / float64(lightsDuration)
)

func lightsTick() tea.Cmd {
	return tea.Tick(lightsFrame, func(time.Time) tea.Msg { return lightsMsg{} })
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
	spin    spinner.Model
	now     func() time.Time
	mode    ViewMode
	lights  float64
}

func New(f Fetcher, login string, mode ViewMode) Model {
	return Model{
		mode:    mode,
		fetcher: f,
		login:   login,
		loading: true,
		width:   twoColumnMinWidth,
		spin:    spinner.New(spinner.WithSpinner(spinner.MiniDot), spinner.WithStyle(sGreen)),
		now:     time.Now,
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
		m.width = msg.Width
	case loadedMsg:
		m.loading = false
		m.user, m.err = msg.user, msg.err
		m.cursor = 0
		if m.err == nil && m.mode == ViewSkyline {
			m.lights = 0
			return m, lightsTick()
		}
		m.lights = 1
	case lightsMsg:
		if m.lights >= 1 {
			return m, nil
		}
		m.lights = min(1, m.lights+lightsStep)
		return m, lightsTick()
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
			return m, nil
		}
		m.mode = ViewSkyline
		m.lights = 0 // replay the city lighting up
		return m, lightsTick()
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
		content = Dashboard(m.user, m.cursor, m.width, m.now(), Options{Mode: m.mode, Lights: m.lights})
		help := "↑/↓ move · enter open repo · p profile · v city/grid · r refresh · q quit"
		if m.loading {
			help = m.spin.View() + " refreshing…"
		}
		content += "\n\n" + sMuted.Render(help)
	}
	v := tea.NewView(content)
	v.AltScreen = true
	v.WindowTitle = "ghscope · @" + m.login
	return v
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
