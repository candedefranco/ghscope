package github

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const okBody = `{"data":{"user":{
  "login":"octo","name":"Octo Cat","bio":"hi","location":"BA","url":"https://github.com/octo",
  "followers":{"totalCount":7},
  "contributionsCollection":{"contributionCalendar":{"totalContributions":3,
    "weeks":[{"contributionDays":[{"date":"2026-10-12","contributionCount":1},{"date":"2026-10-13","contributionCount":2}]}]}},
  "repositories":{"totalCount":1,"nodes":[{
    "name":"palm-blast","description":"fx","url":"https://github.com/octo/palm-blast",
    "stargazerCount":4,"forkCount":1,"pushedAt":"2026-10-10T12:00:00Z","isArchived":false,
    "primaryLanguage":{"name":"HTML","color":"#e34c26"},
    "languages":{"edges":[{"size":1200,"node":{"name":"HTML","color":"#e34c26"}}]}}]}
}}}`

func server(t *testing.T, status int, body string, check func(*http.Request, map[string]any)) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if check != nil {
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Errorf("request body is not JSON: %v", err)
			}
			check(r, payload)
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	c := NewClient("secret-token")
	c.Endpoint = srv.URL
	return c
}

func TestFetchUserParsesProfile(t *testing.T) {
	c := server(t, http.StatusOK, okBody, nil)
	u, err := c.FetchUser(context.Background(), "octo")
	if err != nil {
		t.Fatal(err)
	}
	if u.Name != "Octo Cat" || u.Followers.TotalCount != 7 {
		t.Errorf("profile = %q, %d followers", u.Name, u.Followers.TotalCount)
	}
	if got := u.Contributions.Calendar.Weeks[0].Days[1].Count; got != 2 {
		t.Errorf("day count = %d, want 2", got)
	}
	repo := u.Repositories.Nodes[0]
	if repo.Stars != 4 || repo.PrimaryLanguage.Name != "HTML" || repo.PushedAt.Day() != 10 {
		t.Errorf("repo parsed wrong: %+v", repo)
	}
	if repo.Languages.Edges[0].Size != 1200 {
		t.Errorf("language size = %d", repo.Languages.Edges[0].Size)
	}
}

func TestFetchUserSendsTokenAndLogin(t *testing.T) {
	c := server(t, http.StatusOK, okBody, func(r *http.Request, payload map[string]any) {
		if got := r.Header.Get("Authorization"); got != "bearer secret-token" {
			t.Errorf("Authorization = %q", got)
		}
		vars, _ := payload["variables"].(map[string]any)
		if vars["login"] != "octo" {
			t.Errorf("login variable = %v", vars["login"])
		}
		if q, _ := payload["query"].(string); !strings.Contains(q, "privacy: PUBLIC") {
			t.Error("query must only ask for public repositories")
		}
	})
	if _, err := c.FetchUser(context.Background(), "octo"); err != nil {
		t.Fatal(err)
	}
}

func TestFetchUserErrors(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		want   error
		text   string
	}{
		{"bad token", http.StatusUnauthorized, `{"message":"Bad credentials"}`, ErrUnauthorized, ""},
		{"unknown user", http.StatusOK, `{"data":{"user":null},"errors":[{"type":"NOT_FOUND","message":"Could not resolve"}]}`, ErrNotFound, ""},
		{"null user", http.StatusOK, `{"data":{"user":null}}`, ErrNotFound, ""},
		{"graphql error", http.StatusOK, `{"errors":[{"type":"RATE_LIMITED","message":"slow down"}]}`, nil, "slow down"},
		{"server error", http.StatusBadGateway, `upstream down`, nil, "502"},
		{"bad json", http.StatusOK, `{"data":`, nil, "unexpected response"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := server(t, tc.status, tc.body, nil).FetchUser(context.Background(), "octo")
			if err == nil {
				t.Fatal("expected an error")
			}
			if tc.want != nil && !errors.Is(err, tc.want) {
				t.Errorf("err = %v, want %v", err, tc.want)
			}
			if tc.text != "" && !strings.Contains(err.Error(), tc.text) {
				t.Errorf("err = %q, want it to mention %q", err, tc.text)
			}
		})
	}
}

func TestViewer(t *testing.T) {
	c := server(t, http.StatusOK, `{"data":{"viewer":{"login":"octo"}}}`, func(_ *http.Request, payload map[string]any) {
		if q, _ := payload["query"].(string); !strings.Contains(q, "viewer") {
			t.Errorf("query = %q", q)
		}
	})
	login, err := c.Viewer(context.Background())
	if err != nil || login != "octo" {
		t.Fatalf("Viewer = %q, %v", login, err)
	}
	if _, err := server(t, http.StatusUnauthorized, ``, nil).Viewer(context.Background()); !errors.Is(err, ErrUnauthorized) {
		t.Errorf("err = %v, want ErrUnauthorized", err)
	}
}
