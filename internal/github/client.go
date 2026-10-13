// Package github fetches a user's public profile, contribution calendar and
// repositories from the GitHub GraphQL API in a single request.
package github

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

const DefaultEndpoint = "https://api.github.com/graphql"

// ErrNotFound is returned when the user doesn't exist.
var ErrNotFound = errors.New("user not found")

// ErrUnauthorized is returned when the token is missing, invalid or expired.
var ErrUnauthorized = errors.New("GitHub rejected the token")

type Language struct {
	Name  string `json:"name"`
	Color string `json:"color"` // hex, may be empty
}

type LanguageEdge struct {
	Size int64    `json:"size"` // bytes of code in this language
	Node Language `json:"node"`
}

type Repo struct {
	Name            string    `json:"name"`
	Description     string    `json:"description"`
	URL             string    `json:"url"`
	Stars           int       `json:"stargazerCount"`
	Forks           int       `json:"forkCount"`
	PushedAt        time.Time `json:"pushedAt"`
	IsArchived      bool      `json:"isArchived"`
	PrimaryLanguage *Language `json:"primaryLanguage"`
	Languages       struct {
		Edges []LanguageEdge `json:"edges"`
	} `json:"languages"`
}

type Day struct {
	Date  string `json:"date"` // YYYY-MM-DD
	Count int    `json:"contributionCount"`
}

type Week struct {
	Days []Day `json:"contributionDays"`
}

type Calendar struct {
	Total int    `json:"totalContributions"`
	Weeks []Week `json:"weeks"`
}

type User struct {
	Login     string `json:"login"`
	Name      string `json:"name"`
	Bio       string `json:"bio"`
	Location  string `json:"location"`
	URL       string `json:"url"`
	Followers struct {
		TotalCount int `json:"totalCount"`
	} `json:"followers"`
	Contributions struct {
		Calendar Calendar `json:"contributionCalendar"`
	} `json:"contributionsCollection"`
	Repositories struct {
		TotalCount int    `json:"totalCount"`
		Nodes      []Repo `json:"nodes"`
	} `json:"repositories"`
}

// Only public, non-fork repositories the user owns: private repos never show up,
// even when the token could see them.
const query = `query($login: String!) {
  user(login: $login) {
    login name bio location url
    followers { totalCount }
    contributionsCollection {
      contributionCalendar {
        totalContributions
        weeks { contributionDays { date contributionCount } }
      }
    }
    repositories(first: 100, privacy: PUBLIC, ownerAffiliations: OWNER, isFork: false,
                 orderBy: {field: PUSHED_AT, direction: DESC}) {
      totalCount
      nodes {
        name description url stargazerCount forkCount pushedAt isArchived
        primaryLanguage { name color }
        languages(first: 10, orderBy: {field: SIZE, direction: DESC}) {
          edges { size node { name color } }
        }
      }
    }
  }
}`

// Client talks to the GitHub GraphQL API.
type Client struct {
	Token    string
	Endpoint string
	HTTP     *http.Client
}

func NewClient(token string) *Client {
	return &Client{Token: token, Endpoint: DefaultEndpoint, HTTP: &http.Client{Timeout: 15 * time.Second}}
}

type gqlError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// do runs a GraphQL query and decodes its "data" field into out.
func (c *Client) do(ctx context.Context, q string, vars map[string]string, out any) ([]gqlError, error) {
	body, err := json.Marshal(map[string]any{"query": q, "variables": vars})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "bearer "+c.Token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "ghscope")

	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("could not reach GitHub: %w", err)
	}
	defer res.Body.Close()

	switch {
	case res.StatusCode == http.StatusUnauthorized:
		return nil, ErrUnauthorized
	case res.StatusCode != http.StatusOK:
		snippet, _ := io.ReadAll(io.LimitReader(res.Body, 200))
		return nil, fmt.Errorf("GitHub API returned %s: %s", res.Status, bytes.TrimSpace(snippet))
	}

	envelope := struct {
		Data   json.RawMessage `json:"data"`
		Errors []gqlError      `json:"errors"`
	}{}
	if err := json.NewDecoder(res.Body).Decode(&envelope); err != nil {
		return nil, fmt.Errorf("unexpected response from GitHub: %w", err)
	}
	if len(envelope.Data) > 0 && string(envelope.Data) != "null" {
		if err := json.Unmarshal(envelope.Data, out); err != nil {
			return nil, fmt.Errorf("unexpected response from GitHub: %w", err)
		}
	}
	return envelope.Errors, nil
}

// FetchUser returns the public profile, contributions and repositories of login.
func (c *Client) FetchUser(ctx context.Context, login string) (*User, error) {
	var data struct {
		User *User `json:"user"`
	}
	errs, err := c.do(ctx, query, map[string]string{"login": login}, &data)
	if err != nil {
		return nil, err
	}
	for _, e := range errs {
		if e.Type == "NOT_FOUND" {
			return nil, fmt.Errorf("%w: %s", ErrNotFound, login)
		}
	}
	if len(errs) > 0 {
		return nil, fmt.Errorf("GitHub API error: %s", errs[0].Message)
	}
	if data.User == nil {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, login)
	}
	return data.User, nil
}

// Viewer returns the login of the token's owner.
func (c *Client) Viewer(ctx context.Context) (string, error) {
	var data struct {
		Viewer struct {
			Login string `json:"login"`
		} `json:"viewer"`
	}
	errs, err := c.do(ctx, `query { viewer { login } }`, nil, &data)
	if err != nil {
		return "", err
	}
	if len(errs) > 0 {
		return "", fmt.Errorf("GitHub API error: %s", errs[0].Message)
	}
	if data.Viewer.Login == "" {
		return "", errors.New("GitHub didn't say who the token belongs to")
	}
	return data.Viewer.Login, nil
}
