package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// fakeGitHub renvoie toujours les mêmes dépôts et mémorise la dernière requête.
func fakeGitHub(t *testing.T, lastQuery *string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*lastQuery = r.URL.Query().Get("q")
		json.NewEncoder(w).Encode(searchResp{Items: []Repo{
			{FullName: "x/found", Stars: 42}, {FullName: "a/one", Stars: 7},
		}})
	}))
	t.Cleanup(srv.Close)
	t.Setenv("GITHUB_API_URL", srv.URL)
}

// runCmd exécute une commande Bubble Tea et renvoie les messages produits
// (en déroulant les tea.Batch, sans les ticks d'animation).
func runCmd(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		var out []tea.Msg
		for _, c := range batch {
			if c == nil {
				continue
			}
			// on ignore les commandes d'animation (spinner) qui bloqueraient
			if res := runCmdTimeout(c); res != nil {
				out = append(out, res...)
			}
		}
		return out
	}
	return []tea.Msg{msg}
}

func runCmdTimeout(c tea.Cmd) []tea.Msg {
	ch := make(chan []tea.Msg, 1)
	go func() { ch <- runCmd(c) }()
	select {
	case r := <-ch:
		return r
	case <-timeAfter():
		return nil
	}
}

func submitSearch(t *testing.T, m *model, q string) {
	t.Helper()
	typeText(m, "S")
	if !m.searchMode {
		t.Fatal("« S » devrait ouvrir la saisie de recherche")
	}
	m.searchInput.SetValue(q)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	for _, msg := range runCmd(cmd) {
		if _, ok := msg.(searchDoneMsg); ok {
			m.Update(msg)
		}
	}
}

func TestSearchOpensTemporaryTab(t *testing.T) {
	var q string
	fakeGitHub(t, &q)
	m := testModel(t)
	submitSearch(t, m, "rag language:python")

	if q != "rag language:python" {
		t.Fatalf("requête envoyée : %q", q)
	}
	if !m.onSearchTab() || m.searchLoading {
		t.Fatal("l'onglet de recherche devrait être actif et chargé")
	}
	if got := names(m.visible()); len(got) != 2 || got[0] != "x/found" {
		t.Fatalf("résultats : %v", got)
	}
	if len(m.results) != 2 {
		t.Fatal("la recherche ne doit pas modifier les veilles")
	}

	typeText(m, "x")
	if m.search != nil || m.tab >= len(m.cursor) {
		t.Fatal("« x » devrait fermer l'onglet de recherche")
	}
}

func timeAfter() <-chan time.Time { return time.After(2 * time.Second) }

func TestSaveSearchAsWatch(t *testing.T) {
	var q string
	fakeGitHub(t, &q)
	m := testModel(t)
	m.cfgPath = t.TempDir() + "/config.json"
	if err := os.WriteFile(m.cfgPath, []byte(`{"interval":"1h","state_file":"`+m.cfg.StateFile+`","watches":[{"name":"A","language":"go"},{"name":"B","language":"rust"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	submitSearch(t, m, "rag stars:>100")
	typeText(m, "w")

	cfg, err := loadConfig(m.cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Watches) != 3 || cfg.Watches[2].Query != "rag stars:>100" {
		t.Fatalf("veille non ajoutée : %+v", cfg.Watches)
	}
	raw, _ := os.ReadFile(m.cfgPath)
	if !strings.Contains(string(raw), `"query": "rag stars:>100"`) {
		t.Errorf("la requête doit rester lisible dans config.json :\n%s", raw)
	}
	if strings.Contains(string(raw), `"min_stars"`) {
		t.Errorf("les valeurs par défaut ne doivent pas être écrites :\n%s", raw)
	}
	if m.search != nil || len(m.results) != 3 || m.tab != 2 {
		t.Fatalf("la recherche devrait devenir le 3e onglet (tab=%d, results=%d)", m.tab, len(m.results))
	}
	if _, seen := loadState(m.cfg.StateFile).Seen["x/found"]; !seen {
		t.Error("les dépôts de la nouvelle veille devraient être marqués comme vus")
	}

	// Une deuxième fois : refusé, pas de doublon.
	submitSearch(t, m, "rag stars:>100")
	typeText(m, "w")
	if cfg, _ := loadConfig(m.cfgPath); len(cfg.Watches) != 3 {
		t.Fatal("doublon ajouté")
	}
	if !strings.Contains(m.status, "déjà") {
		t.Errorf("statut attendu « déjà une veille », got %q", m.status)
	}
}

func TestSearchHistory(t *testing.T) {
	var q string
	fakeGitHub(t, &q)
	m := testModel(t)
	submitSearch(t, m, "première")
	submitSearch(t, m, "deuxième")
	submitSearch(t, m, "première") // remonte en tête, sans doublon

	if h := loadHistory(historyPath(m.cfg)); len(h) != 2 || h[0] != "première" {
		t.Fatalf("historique enregistré : %v", h)
	}

	typeText(m, "S")
	m.searchInput.SetValue("brouillon")
	up := tea.KeyMsg{Type: tea.KeyUp}
	down := tea.KeyMsg{Type: tea.KeyDown}
	m.Update(up)
	m.Update(up)
	m.Update(up) // déjà au plus ancien
	if v := m.searchInput.Value(); v != "deuxième" {
		t.Fatalf("après ↑↑↑ : %q", v)
	}
	m.Update(down)
	m.Update(down)
	if v := m.searchInput.Value(); v != "brouillon" {
		t.Fatalf("en redescendant, la saisie en cours doit revenir : %q", v)
	}
}

func TestPushHistoryLimit(t *testing.T) {
	var h []string
	for i := 0; i < historyMax+5; i++ {
		h = pushHistory(h, string(rune('a'+i%26))+strings.Repeat("x", i))
	}
	if len(h) != historyMax {
		t.Fatalf("taille = %d, attendu %d", len(h), historyMax)
	}
}
