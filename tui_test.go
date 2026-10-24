package main

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func testModel(t *testing.T) *model {
	t.Helper()
	cfg := &Config{
		StateFile: t.TempDir() + "/state.json",
		Watches:   []Watch{{Name: "A", Language: "go"}, {Name: "B", Language: "rust"}},
	}
	m := &model{cfg: cfg, interval: time.Hour, cfgPath: "inexistant.json", filter: newFilterInput()}
	m.resetCursors()
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	now := time.Now()
	m.Update(fetchedMsg{at: now, results: []Result{
		{Watch: cfg.Watches[0], Items: []Item{
			{Repo: Repo{FullName: "a/one", Stars: 10}, New: true},
			{Repo: Repo{FullName: "a/two", Stars: 500}},
			{Repo: Repo{FullName: "a/three", Stars: 50}, New: true},
		}},
		{Watch: cfg.Watches[1], Items: []Item{{Repo: Repo{FullName: "b/one"}}}},
	}})
	return m
}

func TestHumanDur(t *testing.T) {
	cases := map[time.Duration]string{
		45 * time.Second:            "45 s",
		90 * time.Second:            "1 min 30 s",
		2*time.Hour + 5*time.Minute: "2 h 05 min",
		24 * time.Hour:              "1 j",
		26 * time.Hour:              "1 j 2 h",
		-time.Minute:                "0 s",
	}
	for d, want := range cases {
		if got := humanDur(d); got != want {
			t.Errorf("humanDur(%v) = %q, attendu %q", d, got, want)
		}
	}
}

func TestNavigationAndTabs(t *testing.T) {
	m := testModel(t)
	m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m.Update(tea.KeyMsg{Type: tea.KeyDown}) // bloqué en bas
	if m.cursor[0] != 2 {
		t.Fatalf("curseur = %d, attendu 2", m.cursor[0])
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRight})
	if m.tab != 1 {
		t.Fatalf("onglet = %d, attendu 1", m.tab)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRight}) // reboucle
	if m.tab != 0 || m.cursor[0] != 2 {
		t.Fatalf("onglet=%d curseur=%d : la position devrait être conservée", m.tab, m.cursor[0])
	}
}

func TestAutoRefreshWhenIntervalElapsed(t *testing.T) {
	m := testModel(t)
	m.Update(clockMsg(m.nextRun.Add(-time.Second)))
	if m.loading {
		t.Fatal("ne devrait pas rafraîchir avant l'échéance")
	}
	_, cmd := m.Update(clockMsg(m.nextRun.Add(time.Second)))
	if !m.loading || cmd == nil {
		t.Fatal("devrait lancer une recherche une fois l'intervalle écoulé")
	}
}

func typeText(m *model, s string) {
	for _, r := range s {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
}

func names(items []Item) []string {
	var out []string
	for _, it := range items {
		out = append(out, it.FullName)
	}
	return out
}

func TestSortFilterNewOnly(t *testing.T) {
	m := testModel(t)
	if got := names(m.visible()); got[0] != "a/two" {
		t.Fatalf("tri par étoiles attendu, got %v", got)
	}
	typeText(m, "s") // → plus récents
	typeText(m, "s") // → nom
	if got := names(m.visible()); got[0] != "a/one" || got[2] != "a/two" {
		t.Fatalf("tri par nom attendu, got %v", got)
	}
	typeText(m, "n")
	if got := names(m.visible()); len(got) != 2 {
		t.Fatalf("nouveautés uniquement : attendu 2, got %v", got)
	}
	typeText(m, "n")

	typeText(m, "/")
	typeText(m, "thr")
	if !m.filtering {
		t.Fatal("devrait être en saisie de filtre")
	}
	if got := names(m.visible()); len(got) != 1 || got[0] != "a/three" {
		t.Fatalf("filtre « thr » : got %v", got)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	typeText(m, "q") // hors saisie, q quitte : ne doit pas s'ajouter au filtre
	if m.filter.Value() != "thr" {
		t.Fatalf("filtre modifié après validation : %q", m.filter.Value())
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if len(m.visible()) != 3 {
		t.Fatal("esc devrait effacer le filtre")
	}
}
