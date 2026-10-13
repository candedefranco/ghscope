package github

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"time"
)

// ErrNoToken means no GitHub token could be found.
var ErrNoToken = errors.New("no GitHub token found")

// ResolveToken looks for a token in GITHUB_TOKEN, then GH_TOKEN, then asks the
// GitHub CLI (`gh auth token`) if it's installed and logged in.
func ResolveToken(ctx context.Context) (string, error) {
	for _, env := range []string{"GITHUB_TOKEN", "GH_TOKEN"} {
		if t := strings.TrimSpace(os.Getenv(env)); t != "" {
			return t, nil
		}
	}
	if _, err := exec.LookPath("gh"); err != nil {
		return "", ErrNoToken
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "gh", "auth", "token").Output()
	if t := strings.TrimSpace(string(out)); err == nil && t != "" {
		return t, nil
	}
	return "", ErrNoToken
}
