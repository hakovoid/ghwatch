package main

import (
	"fmt"
	"io"
	"log"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

const itemHeight = 3 // 2 lignes + 1 ligne d'espace

type statusMsg string

type fetchedMsg struct {
	results []Result
	at      time.Time
}

type model struct {
	cfgPath, intervalOverride, token string
	cfg                              *Config
	interval                         time.Duration

	results []Result
	tab     int
	cursor  []int // position du curseur par onglet
	offset  []int // défilement par onglet

	loading bool
	spin    spinner.Model
	lastRun time.Time

	width, height int
	status        string
}

func runTUI(cfgPath, intervalOverride, token string) error {
	cfg, interval, err := loadConfigWithInterval(cfgPath, intervalOverride)
	if err != nil {
		return err
	}
	log.SetOutput(io.Discard) // les logs casseraient l'affichage

	sp := spinner.New()
	sp.Spinner = spinner.MiniDot
	sp.Style = lipgloss.NewStyle().Foreground(cAccent)

	m := &model{
		cfgPath: cfgPath, intervalOverride: intervalOverride, token: token,
		cfg: cfg, interval: interval, spin: sp, loading: true,
	}
	m.resetCursors()
	_, err = tea.NewProgram(m, tea.WithAltScreen()).Run()
	return err
}

func (m *model) resetCursors() {
	n := len(m.cfg.Watches)
	if m.results != nil {
		n = len(m.results)
	}
	if len(m.cursor) != n {
		m.cursor = make([]int, n)
		m.offset = make([]int, n)
	}
	if m.tab >= n {
		m.tab = 0
	}
}

func fetchCmd(cfg *Config, token string) tea.Cmd {
	return func() tea.Msg {
		now := time.Now()
		return fetchedMsg{results: Fetch(cfg, token, now), at: now}
	}
}

func (m *model) Init() tea.Cmd {
	return tea.Batch(m.spin.Tick, fetchCmd(m.cfg, m.token))
}

// visible renvoie les dépôts affichés dans l'onglet courant.
func (m *model) visible() []Item {
	if m.tab >= len(m.results) {
		return nil
	}
	return m.results[m.tab].Items
}

// selected renvoie le dépôt sous le curseur.
func (m *model) selected() (Item, bool) {
	items := m.visible()
	if len(items) == 0 || m.tab >= len(m.cursor) {
		return Item{}, false
	}
	return items[min(m.cursor[m.tab], len(items)-1)], true
}

func (m *model) listHeight() int {
	return max(m.height-5, itemHeight) // barre de titre, onglets, règle, pied
}

func (m *model) perPage() int { return max(m.listHeight()/itemHeight, 1) }

func (m *model) moveCursor(delta int) {
	items := m.visible()
	if len(items) == 0 {
		return
	}
	c := min(max(m.cursor[m.tab]+delta, 0), len(items)-1)
	m.cursor[m.tab] = c
	pp := m.perPage()
	if c < m.offset[m.tab] {
		m.offset[m.tab] = c
	} else if c >= m.offset[m.tab]+pp {
		m.offset[m.tab] = c - pp + 1
	}
}

func (m *model) switchTab(delta int) {
	n := len(m.cursor)
	if n == 0 {
		return
	}
	m.tab = (m.tab + delta + n) % n
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.moveCursor(0)

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd

	case fetchedMsg:
		m.results, m.lastRun, m.loading = msg.results, msg.at, false
		m.resetCursors()
		m.moveCursor(0)

	case statusMsg:
		m.status = string(msg)

	case tea.KeyMsg:
		m.status = ""
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "enter", "o":
			if it, ok := m.selected(); ok {
				url := it.HTMLURL
				return m, func() tea.Msg {
					if err := openURL(url); err != nil {
						return statusMsg("impossible d'ouvrir le navigateur : " + err.Error())
					}
					return statusMsg("ouvert : " + url)
				}
			}
		case "up", "k":
			m.moveCursor(-1)
		case "down", "j":
			m.moveCursor(1)
		case "pgup":
			m.moveCursor(-m.perPage())
		case "pgdown", " ":
			m.moveCursor(m.perPage())
		case "home", "g":
			m.moveCursor(-1 << 30)
		case "end", "G":
			m.moveCursor(1 << 30)
		case "right", "l", "tab":
			m.switchTab(1)
		case "left", "h", "shift+tab":
			m.switchTab(-1)
		}
	}
	return m, nil
}

// ---------- Rendu ----------

func (m *model) View() string {
	if m.width == 0 {
		return ""
	}
	header := m.viewHeader()
	tabs := m.viewTabs()
	rule := sRule.Render(strings.Repeat("─", m.width))
	footer := m.viewFooter()
	h := m.listHeight()
	var body string
	if m.width >= 100 { // assez large : liste à gauche, détail à droite
		lw := m.width * 56 / 100
		list := lipgloss.NewStyle().Width(lw).Height(h).MaxHeight(h).Render(m.viewList(lw - 1))
		body = lipgloss.JoinHorizontal(lipgloss.Top, list, m.viewDetail(m.width-lw, h))
	} else {
		body = lipgloss.NewStyle().Height(h).MaxHeight(h).Render(m.viewList(m.width))
	}
	return lipgloss.JoinVertical(lipgloss.Left, header, tabs, rule, body, footer)
}

func (m *model) viewHeader() string {
	left := sLogo.Render("◆ ghwatch") + sBar.Render("  veille GitHub ")
	var right string
	switch {
	case m.loading:
		right = m.spin.View() + " recherche en cours…"
	case !m.lastRun.IsZero():
		right = "vérifié à " + m.lastRun.Format("15:04")
	}
	right = sBar.Render(right + " ")
	gap := max(m.width-lipgloss.Width(left)-lipgloss.Width(right), 0)
	return left + sBar.Render(strings.Repeat(" ", gap)) + right
}

func (m *model) viewTabs() string {
	var tabs []string
	for i, w := range m.cfg.Watches {
		label := w.Name
		if i < len(m.results) {
			label = m.results[i].Watch.Name
			if n := m.results[i].NewCount(); n > 0 {
				badge := sTabBadge
				if i == m.tab { // lisible sur le fond violet de l'onglet actif
					badge = badge.Foreground(cOnColor).Background(cAccent)
				}
				label += " " + badge.Render(fmt.Sprintf("●%d", n))
			}
		}
		if i == m.tab {
			tabs = append(tabs, sTabActive.Render(label))
		} else {
			tabs = append(tabs, sTabInactive.Render(label))
		}
	}
	line := lipgloss.JoinHorizontal(lipgloss.Top, tabs...)
	return ansi.Truncate(line, m.width, "…")
}

func (m *model) viewList(width int) string {
	if m.loading && m.results == nil {
		return "\n  " + m.spin.View() + sMuted.Render(" Interrogation de GitHub…")
	}
	if m.tab < len(m.results) {
		if err := m.results[m.tab].Err; err != nil {
			msg := ansi.Wordwrap(err.Error(), width-6, " ")
			return "\n  " + sErr.Render("⚠  Erreur") + "\n\n" +
				lipgloss.NewStyle().PaddingLeft(2).Foreground(cMuted).Render(msg)
		}
	}
	items := m.visible()
	if len(items) == 0 {
		return "\n  " + sMuted.Render("Aucun dépôt pour cette veille.")
	}

	var b strings.Builder
	start := m.offset[m.tab]
	end := min(start+m.perPage(), len(items))
	for i := start; i < end; i++ {
		b.WriteString(m.viewItem(items[i], i == m.cursor[m.tab], width))
		if i < end-1 {
			b.WriteString("\n\n")
		}
	}
	return b.String()
}

func (m *model) viewItem(it Item, selected bool, width int) string {
	cursor := "  "
	repoStyle := sRepo
	if selected {
		cursor = sCursor.Render("▌ ")
		repoStyle = sRepoSel
	}

	owner, name, _ := strings.Cut(it.FullName, "/")
	title := sOwner.Render(owner+"/") + repoStyle.Render(name)
	if it.New {
		title += " " + sNewBadge.Render("NEW")
	}
	stars := sStars.Render("★ " + humanStars(it.Stars))
	gap := max(width-lipgloss.Width(cursor)-lipgloss.Width(title)-lipgloss.Width(stars)-1, 1)
	line1 := cursor + title + strings.Repeat(" ", gap) + stars

	desc := strings.TrimSpace(it.Description)
	if desc == "" {
		desc = "pas de description"
	}
	meta := langBadge(it.Language) + sMuted.Render(" · "+relTime(it.CreatedAt)+" · ")
	line2 := cursor + meta + sDesc.Render(desc)

	return ansi.Truncate(line1, width, "") + "\n" + ansi.Truncate(line2, width-1, "…")
}

func (m *model) viewDetail(width, height int) string {
	box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(cFaint).
		Padding(0, 2).Width(width - 2).Height(height - 2).MaxHeight(height)
	it, ok := m.selected()
	if !ok {
		return box.Render("")
	}
	inner := width - 6

	owner, name, _ := strings.Cut(it.FullName, "/")
	title := sOwner.Render(owner+"/") + sRepoSel.Render(name)
	if it.New {
		title += " " + sNewBadge.Render("NEW")
	}

	desc := strings.TrimSpace(it.Description)
	if desc == "" {
		desc = sMuted.Render("Pas de description.")
	}
	desc = sDesc.Render(ansi.Wordwrap(desc, inner, " "))

	stats := sStars.Render("★ "+humanStars(it.Stars)) + sMuted.Render("   ⑂ "+humanStars(it.Forks)+
		"   ◎ "+humanStars(it.Issues)+" issues")

	license := "—"
	if it.License != nil && it.License.SPDX != "" && it.License.SPDX != "NOASSERTION" {
		license = it.License.SPDX
	}
	row := func(k, v string) string {
		return sMuted.Render(fmt.Sprintf("%-15s", k)) + v
	}
	facts := strings.Join([]string{
		row("Langage", langBadge(it.Language)),
		row("Licence", sDesc.Render(license)),
		row("Créé", sDesc.Render(it.CreatedAt.Format("02/01/2006"))+sMuted.Render("  ("+relTime(it.CreatedAt)+")")),
		row("Dernier commit", sDesc.Render(relTime(it.PushedAt))),
	}, "\n")

	parts := []string{title, "", desc, "", stats, "", facts}

	if len(it.Topics) > 0 {
		var chips []string
		lineW := 0
		var lines []string
		for _, t := range it.Topics {
			c := sTopic.Render(t)
			if lineW > 0 && lineW+lipgloss.Width(c)+1 > inner {
				lines = append(lines, strings.Join(chips, " "))
				chips, lineW = nil, 0
			}
			chips = append(chips, c)
			lineW += lipgloss.Width(c) + 1
		}
		lines = append(lines, strings.Join(chips, " "))
		parts = append(parts, "", strings.Join(lines, "\n"))
	}

	link := lipgloss.NewStyle().Foreground(cAccent2).Underline(true).Render(ansi.Truncate(it.HTMLURL, inner, "…"))
	parts = append(parts, "", link, sMuted.Render("⏎ ouvrir dans le navigateur"))

	return box.Render(strings.Join(parts, "\n"))
}

func (m *model) viewFooter() string {
	if m.status != "" {
		return ansi.Truncate(" "+sKey.Render("›")+" "+sDesc.Render(m.status), m.width, "…")
	}
	keys := []struct{ k, v string }{
		{"↑↓", "naviguer"}, {"←→", "veilles"}, {"⏎", "ouvrir"}, {"q", "quitter"},
	}
	var parts []string
	for _, k := range keys {
		parts = append(parts, sKey.Render(k.k)+" "+sMuted.Render(k.v))
	}
	help := " " + strings.Join(parts, sRule.Render("  •  "))

	pos := ""
	if items := m.visible(); len(items) > 0 {
		pos = sMuted.Render(fmt.Sprintf("%d/%d ", m.cursor[m.tab]+1, len(items)))
	}
	gap := max(m.width-lipgloss.Width(help)-lipgloss.Width(pos), 1)
	return ansi.Truncate(help+strings.Repeat(" ", gap)+pos, m.width, "")
}

// relTime : « aujourd'hui », « il y a 3 j », « il y a 2 mois »…
func relTime(t time.Time) string {
	if t.IsZero() {
		return "date inconnue"
	}
	d := time.Since(t)
	days := int(d.Hours() / 24)
	switch {
	case d < time.Hour:
		return "il y a quelques minutes"
	case d < 24*time.Hour:
		return fmt.Sprintf("il y a %d h", int(d.Hours()))
	case days < 30:
		return fmt.Sprintf("il y a %d j", days)
	case days < 365:
		return fmt.Sprintf("il y a %d mois", days/30)
	}
	return fmt.Sprintf("il y a %d an(s)", days/365)
}
