package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestBuildQueriesAny(t *testing.T) {
	now := time.Date(2026, 10, 24, 9, 0, 0, 0, time.UTC)
	w := Watch{Any: []string{"topic:llm", " claude in:name,description ", ""}, MinStars: 50, CreatedWithin: "7d"}
	qs, err := buildQueries(w, now)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"topic:llm stars:>=50 created:>=2026-10-17",
		"claude in:name,description stars:>=50 created:>=2026-10-17",
	}
	if strings.Join(qs, "\n") != strings.Join(want, "\n") {
		t.Fatalf("got %q", qs)
	}

	// « any » seul, sans autre critère : accepté.
	if qs, err := buildQueries(Watch{Any: []string{"topic:mcp"}}, now); err != nil || qs[0] != "topic:mcp" {
		t.Fatalf("any seul : %q, %v", qs, err)
	}
	// Ni critère ni « any » : refusé.
	if _, err := buildQueries(Watch{Name: "vide"}, now); err == nil {
		t.Fatal("une veille sans critère devrait être refusée")
	}
}

func TestFetchAnyMergesDedupesAndSorts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		var items []Repo
		switch {
		case strings.HasPrefix(q, "topic:llm"):
			items = []Repo{{FullName: "g/graphify", Stars: 900}, {FullName: "x/small", Stars: 5}}
		case strings.HasPrefix(q, "topic:claude-code"):
			items = []Repo{{FullName: "g/graphify", Stars: 900}, {FullName: "t/archify", Stars: 400}}
		case strings.HasPrefix(q, "topic:broken"):
			w.WriteHeader(http.StatusUnprocessableEntity)
			w.Write([]byte(`{"message":"Validation Failed"}`))
			return
		}
		json.NewEncoder(w).Encode(searchResp{Items: items})
	}))
	defer srv.Close()
	t.Setenv("GITHUB_API_URL", srv.URL)
	searchPause = 0

	cfg := &Config{StateFile: t.TempDir() + "/s.json", Watches: []Watch{{
		Name: "IA", Any: []string{"topic:llm", "topic:claude-code", "topic:broken"}, Sort: "stars", Limit: 2,
	}}}
	res := Fetch(cfg, "", time.Now())[0]

	if res.Err != nil {
		t.Fatalf("un échec partiel ne doit pas faire échouer la veille : %v", res.Err)
	}
	if res.Failed != 1 {
		t.Errorf("Failed = %d, attendu 1", res.Failed)
	}
	var got []string
	for _, it := range res.Items {
		got = append(got, it.FullName)
	}
	if strings.Join(got, ",") != "g/graphify,t/archify" {
		t.Fatalf("fusion/tri/limite incorrects : %v", got)
	}
	if !strings.Contains(res.Query, "  |  ") {
		t.Errorf("la requête affichée doit lister toutes les recherches : %q", res.Query)
	}

	// Toutes les requêtes en échec : erreur.
	cfg.Watches[0].Any = []string{"topic:broken"}
	if r := Fetch(cfg, "", time.Now())[0]; r.Err == nil {
		t.Fatal("attendu une erreur quand toutes les requêtes échouent")
	}
}
