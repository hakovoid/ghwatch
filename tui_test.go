package main

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func testModel(t *testing.T) *model {
	t.Helper()
	cfg := &Config{
		StateFile: t.TempDir() + "/state.json",
		Watches:   []Watch{{Name: "A", Language: "go"}, {Name: "B", Language: "rust"}},
	}
	m := newModel(cfg, time.Hour)
	m.cfgPath = "inexistant.json"
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
	m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m.Update(tea.KeyMsg{Type: tea.KeyRight})
	if m.tab != m.markTab(markLater) {
		t.Fatalf("onglet = %d, attendu « À voir » après les veilles et les favoris", m.tab)
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
	typeText(m, "s") // → progression
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

func TestHelpToggle(t *testing.T) {
	m := testModel(t)
	typeText(m, "?")
	if !m.showHelp || !strings.Contains(m.View(), "Raccourcis clavier") {
		t.Fatal("« ? » devrait afficher l'aide")
	}
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if m.showHelp || cmd != nil {
		t.Fatal("une touche devrait fermer l'aide sans quitter")
	}
}

// Aucune vue ne doit dépasser la taille du terminal, quelle qu'elle soit.
func TestViewFitsTerminal(t *testing.T) {
	for _, size := range [][2]int{{60, 16}, {80, 24}, {120, 30}, {200, 50}} {
		m := testModel(t)
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		for _, keys := range []string{"", "?", "zS"} { // z ferme l'aide, S ouvre la recherche
			typeText(m, keys)
			m.history = []string{"ancienne recherche"}
			v := m.View()
			if h := lipgloss.Height(v); h > size[1] {
				t.Errorf("%dx%d (touches %q) : hauteur %d", size[0], size[1], keys, h)
			}
			if w := lipgloss.Width(v); w > size[0] {
				t.Errorf("%dx%d (touches %q) : largeur %d", size[0], size[1], keys, w)
			}
		}
	}
}

func TestRepoNamesAreClickable(t *testing.T) {
	m := testModel(t)
	m.results[0].Items[1].HTMLURL = "https://github.com/a/two" // en tête (tri par étoiles)
	v := m.View()
	if !strings.Contains(v, "\x1b]8;;https://github.com/a/two") {
		t.Fatal("le nom du dépôt devrait être un lien OSC 8")
	}
	linksEnabled = false
	defer func() { linksEnabled = true }()
	if strings.Contains(m.View(), "\x1b]8;;") {
		t.Fatal("-links=false devrait désactiver les liens")
	}
}

func TestCopyLink(t *testing.T) {
	var got string
	orig := copyToClipboard
	copyToClipboard = func(s string) string { got = s; return "presse-papiers" }
	defer func() { copyToClipboard = orig }()

	m := testModel(t)
	m.results[0].Items[1].HTMLURL = "https://github.com/a/two"
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	if cmd == nil {
		t.Fatal("la copie doit se faire en tâche de fond")
	}
	m.Update(cmd())
	if got != "https://github.com/a/two" || !strings.Contains(m.status, "copié") {
		t.Fatalf("copié %q, statut %q", got, m.status)
	}
}

func key(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

// La vue doit tenir dans le terminal et garder la même hauteur, quelle que
// soit la disposition et le partage liste / détail.
func TestLayoutFillsScreen(t *testing.T) {
	m := testModel(t)
	for _, size := range [][2]int{{120, 30}, {80, 40}, {120, 12}, {160, 50}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		want := -1
		for _, side := range []bool{false, true} {
			m.sideBySide = side
			for _, split := range []int{splitMin, splitDefault, splitMax} {
				m.split = split
				got := lipgloss.Height(m.View())
				if want < 0 {
					want = got
				}
				if got != want || got > size[1] {
					t.Errorf("%dx%d côte à côte=%v split=%d : %d lignes", size[0], size[1], side, split, got)
				}
			}
		}
	}
}

func TestStackedByDefault(t *testing.T) {
	m := testModel(t)
	if !m.showStacked() || m.showSideBySide() {
		t.Fatal("le détail doit être sous la liste par défaut")
	}
	m.Update(key("v"))
	if !m.showSideBySide() {
		t.Fatal("v doit passer en vue côte à côte")
	}
}

func TestResizeClamps(t *testing.T) {
	m := testModel(t)
	for range 10 {
		m.Update(key("+"))
	}
	if m.split != splitMax {
		t.Errorf("split = %d, attendu %d", m.split, splitMax)
	}
	for range 10 {
		m.Update(key("-"))
	}
	if m.split != splitMin {
		t.Errorf("split = %d, attendu %d", m.split, splitMin)
	}
	m.Update(key("="))
	if m.split != splitDefault {
		t.Errorf("split = %d, attendu %d", m.split, splitDefault)
	}
}

func TestDisplayTextStripsEmoji(t *testing.T) {
	cases := map[string]string{
		"🚀 Fast ✨ agent ❤️ by 👨‍💻 devs 🇫🇷": "Fast agent by devs",
		"中文 description — accentuée":       "中文 description — accentuée",
		"  indent 🔥 kept\nline ⚡ two":      "  indent kept\nline two",
	}
	for in, want := range cases {
		if got := displayText(in); got != want {
			t.Errorf("displayText(%q) = %q, attendu %q", in, got, want)
		}
	}
}

// Aucune ligne ne doit dépasser la largeur du terminal, quels que soient
// l'état de l'interface et le contenu (emoji, CJK, noms très longs).
func TestViewNeverOverflows(t *testing.T) {
	m := testModel(t)
	m.results[0].Items[0].Description = "🚀 让你的agent一键直出商业级产品宣传片 " + strings.Repeat("très long ", 30)
	m.results[0].Items[1].FullName = strings.Repeat("x", 80) + "/" + strings.Repeat("y", 80)
	states := []func(){
		func() {}, func() { m.showHelp = true }, func() { m.filtering = true },
		func() { m.searchMode = true }, func() { m.loading = true }, func() { m.sideBySide = true },
		func() { m.status = strings.Repeat("statut ", 40) },
	}
	for si, set := range states {
		for w := 30; w <= 200; w += 17 {
			for h := 6; h <= 50; h += 11 {
				m.showHelp, m.filtering, m.searchMode, m.loading, m.sideBySide, m.status = false, false, false, false, false, ""
				m.Update(tea.WindowSizeMsg{Width: w, Height: h})
				set()
				for tab := 0; tab < len(m.cursor); tab++ {
					m.tab = tab
					lines := strings.Split(m.View(), "\n")
					if len(lines) > h {
						t.Fatalf("état %d, %dx%d, onglet %d : %d lignes", si, w, h, tab, len(lines))
					}
					for i, l := range lines {
						if lw := lipgloss.Width(l); lw > w {
							t.Fatalf("état %d, %dx%d, onglet %d, ligne %d : largeur %d", si, w, h, tab, i, lw)
						}
					}
				}
			}
		}
	}
}

func TestThemeCycleIsRemembered(t *testing.T) {
	m := testModel(t)
	defer applyTheme(themes[0].name)
	first := m.theme
	m.Update(key("T"))
	if m.theme == first || m.st.prefs().Theme != m.theme {
		t.Fatalf("thème %q → %q, mémorisé %q", first, m.theme, m.st.prefs().Theme)
	}
	m.Update(key("v"))
	m.Update(key("+"))
	if p := m.st.prefs(); !p.SideBySide || p.Split != splitDefault+splitStep {
		t.Fatalf("disposition non mémorisée : %+v", p)
	}
	_ = m.st.save(m.cfg.StateFile)
	m2 := newModel(m.cfg, time.Hour)
	if !m2.sideBySide || m2.split != splitDefault+splitStep {
		t.Fatalf("disposition non relue : côte à côte=%v split=%d", m2.sideBySide, m2.split)
	}
	for _, n := range themeNames() {
		if applyTheme(n) != n {
			t.Errorf("thème %q non appliqué", n)
		}
	}
}
