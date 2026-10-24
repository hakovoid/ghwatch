package main

import (
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Recherche GitHub ponctuelle depuis la TUI : les résultats s'affichent dans
// un onglet temporaire, sans toucher aux veilles ni à l'état « déjà vu ».

const searchLimit = 30

type searchDoneMsg struct {
	result Result
	at     time.Time
}

func newSearchInput() textinput.Model {
	ti := textinput.New()
	ti.Prompt = "GitHub › "
	ti.Placeholder = "ex : rag language:python stars:>100 created:>2026-09-01"
	ti.PromptStyle = sKey
	ti.TextStyle = sDesc
	ti.PlaceholderStyle = sMuted
	ti.Cursor.Style = lipgloss.NewStyle().Foreground(cAccent2)
	ti.CharLimit = 200
	return ti
}

// searchCmd lance la recherche. Les dépôts jamais vus par tes veilles sont
// marqués NEW, mais l'état n'est pas modifié : une recherche ponctuelle ne
// doit pas « consommer » les nouveautés de tes veilles.
func searchCmd(cfg *Config, query, token string) tea.Cmd {
	return func() tea.Msg {
		now := time.Now()
		state := loadState(cfg.StateFile)
		repos, err := searchRepos(query, "stars", searchLimit, token)
		r := Result{Watch: Watch{Name: query, Query: query, Limit: 10}, Query: query, Err: err}
		for _, repo := range repos {
			_, seen := state.Seen[repo.FullName]
			r.Items = append(r.Items, Item{Repo: repo, New: !seen})
		}
		return searchDoneMsg{result: r, at: now}
	}
}

// watchTabs : nombre d'onglets de veilles (hors onglet de recherche).
func (m *model) watchTabs() int {
	if m.results != nil {
		return len(m.results)
	}
	return len(m.cfg.Watches)
}

// tabs renvoie les résultats de tous les onglets, recherche comprise (en dernier).
func (m *model) tabs() []Result {
	if m.search == nil {
		return m.results
	}
	out := make([]Result, 0, len(m.results)+1)
	out = append(out, m.results...)
	// Avant le premier chargement, on complète avec des onglets vides pour
	// que l'onglet de recherche garde son index.
	for i := len(m.results); i < m.watchTabs(); i++ {
		out = append(out, Result{Watch: m.cfg.Watches[i]})
	}
	return append(out, *m.search)
}

func (m *model) onSearchTab() bool { return m.search != nil && m.tab == m.watchTabs() }

// updateSearchInput gère la barre de saisie de la recherche.
func (m *model) updateSearchInput(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "esc":
		m.searchMode = false
		m.searchInput.Blur()
		return nil
	case "ctrl+c":
		return tea.Quit
	case "enter":
		q := m.searchInput.Value()
		if q == "" {
			return nil
		}
		m.searchMode = false
		m.searchInput.Blur()
		m.search = &Result{Watch: Watch{Name: q}, Query: q}
		m.searchLoading = true
		m.resetCursors()
		m.tab = m.watchTabs()
		m.cursor[m.tab], m.offset[m.tab] = 0, 0
		return tea.Batch(m.spin.Tick, searchCmd(m.cfg, q, m.token))
	}
	var cmd tea.Cmd
	m.searchInput, cmd = m.searchInput.Update(msg)
	return cmd
}

func (m *model) closeSearch() {
	m.search, m.searchLoading = nil, false
	m.resetCursors()
	if m.tab >= len(m.cursor) {
		m.tab = max(len(m.cursor)-1, 0)
	}
}
