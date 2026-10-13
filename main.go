// ghscope shows a GitHub user's contributions, languages and repositories in
// the terminal.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/candedefranco/ghscope/internal/github"
	"github.com/candedefranco/ghscope/internal/ui"
)

var version = "dev"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "ghscope:", err)
		os.Exit(1)
	}
}

func run() error {
	once := flag.Bool("once", false, "print the dashboard once and exit (no interactive UI)")
	width := flag.Int("width", 110, "layout width for --once")
	viewFlag := flag.String("view", "skyline", "how to draw contributions: skyline or heatmap")
	showVersion := flag.Bool("version", false, "print the version")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: ghscope [flags] [username]\n\n"+
			"Shows a GitHub user's contributions, languages and repositories.\n"+
			"Without a username, shows the user you're logged in as.\n\n"+
			"A token is read from GITHUB_TOKEN, GH_TOKEN or the GitHub CLI (`gh auth token`).\n\nflags:\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	if *showVersion {
		fmt.Println("ghscope", version)
		return nil
	}
	if flag.NArg() > 1 {
		flag.Usage()
		os.Exit(2)
	}

	mode, err := ui.ParseViewMode(*viewFlag)
	if err != nil {
		return err
	}

	ctx := context.Background()
	token, err := github.ResolveToken(ctx)
	if errors.Is(err, github.ErrNoToken) {
		return errors.New("no GitHub token found. Install the GitHub CLI and run `gh auth login`, " +
			"or set GITHUB_TOKEN to a token with no extra scopes (public data only)")
	}
	client := github.NewClient(token)

	login := flag.Arg(0)
	if login == "" {
		login, err = client.Viewer(ctx)
		if err != nil {
			return errors.New(ui.ErrorHint(err))
		}
	}

	if *once {
		ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		u, err := client.FetchUser(ctx, login)
		if err != nil {
			return errors.New(ui.ErrorHint(err))
		}
		// lipgloss.Println adapts colors to the terminal (and strips them when piped)
		_, err = lipgloss.Println(ui.Dashboard(u, 0, *width, time.Now(), ui.Options{Mode: mode, Lights: 1}))
		return err
	}

	_, err = tea.NewProgram(ui.New(client, login, mode)).Run()
	return err
}
