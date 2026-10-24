package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseDuration(t *testing.T) {
	cases := map[string]time.Duration{
		"30m":  30 * time.Minute,
		"6h":   6 * time.Hour,
		"1d":   24 * time.Hour,
		"2w":   14 * 24 * time.Hour,
		"1.5d": 36 * time.Hour,
	}
	for in, want := range cases {
		got, err := parseDuration(in)
		if err != nil || got != want {
			t.Errorf("parseDuration(%q) = %v, %v ; attendu %v", in, got, err, want)
		}
	}
	if _, err := parseDuration("abc"); err == nil {
		t.Error("attendu une erreur pour 'abc'")
	}
}

func TestBuildQuery(t *testing.T) {
	now := time.Date(2026, 10, 24, 9, 0, 0, 0, time.UTC)
	q, err := buildQuery(Watch{Topics: []string{"llm"}, Language: "go", MinStars: 50, CreatedWithin: "7d"}, now)
	if err != nil {
		t.Fatal(err)
	}
	want := "topic:llm language:go stars:>=50 created:>=2026-10-17"
	if q != want {
		t.Errorf("got %q, want %q", q, want)
	}
}

func TestCheckOnlyShowsNewRepos(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/search/repositories" || r.URL.Query().Get("q") == "" {
			t.Errorf("requête inattendue: %s", r.URL)
		}
		items := []Repo{{FullName: "a/one", HTMLURL: "https://github.com/a/one", Stars: 120, Language: "Go"}}
		if calls > 1 {
			items = append(items, Repo{FullName: "b/two", HTMLURL: "https://github.com/b/two", Stars: 300})
		}
		json.NewEncoder(w).Encode(searchResp{TotalCount: len(items), Items: items})
	}))
	defer srv.Close()
	t.Setenv("GITHUB_API_URL", srv.URL)

	dir := t.TempDir()
	cfg := &Config{
		StateFile:  filepath.Join(dir, "state.json"),
		ReportsDir: filepath.Join(dir, "reports"),
		Watches:    []Watch{{Name: "test", Language: "go", Sort: "stars", Limit: 10}},
	}

	stdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	searchPause, searchPauseAnon = 0, 0
	check(cfg, "", false) // 1er passage : a/one est nouveau
	check(cfg, "", false) // 2e passage : seul b/two est nouveau
	w.Close()
	os.Stdout = stdout
	buf := make([]byte, 1<<16)
	n, _ := r.Read(buf)
	out := string(buf[:n])

	parts := strings.Split(out, "# Veille GitHub")
	if len(parts) != 3 {
		t.Fatalf("attendu 2 rapports, sortie:\n%s", out)
	}
	if !strings.Contains(parts[1], "a/one") {
		t.Error("1er rapport devrait contenir a/one")
	}
	if strings.Contains(parts[2], "a/one") || !strings.Contains(parts[2], "b/two") {
		t.Errorf("2e rapport devrait ne contenir que b/two:\n%s", parts[2])
	}
	if files, _ := os.ReadDir(cfg.ReportsDir); len(files) == 0 {
		t.Error("aucun rapport Markdown écrit")
	}
}
