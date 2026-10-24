package main

import (
	"fmt"
	"io"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

const itemHeight = 3 // 2 lignes + 1 ligne d'espace

type sortMode int

const (
	sortStars sortMode = iota
	sortRecent
	sortName
	sortModes
)

func (s sortMode) String() string {
	return [...]string{"étoiles", "plus récents", "nom"}[s]
}

type statusMsg string

type clockMsg time.Time

// clock réveille la TUI chaque seconde (compte à rebours, auto-refresh).
func clock() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg { return clockMsg(t) })
}

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
	nextRun time.Time
	now     time.Time

	width, height int
	status        string

	filter    textinput.Model
	filtering bool // saisie du filtre en cours
	sort      sortMode
	newOnly   bool
	showHelp  bool
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
		filter: newFilterInput(),
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

func newFilterInput() textinput.Model {
	ti := textinput.New()
	ti.Prompt = "⌕ "
	ti.Placeholder = "nom, description, langage, topic…"
	ti.PromptStyle = sKey
	ti.TextStyle = sDesc
	ti.PlaceholderStyle = sMuted
	ti.Cursor.Style = lipgloss.NewStyle().Foreground(cAccent)
	ti.CharLimit = 60
	return ti
}

func fetchCmd(cfg *Config, token string) tea.Cmd {
	return func() tea.Msg {
		now := time.Now()
		return fetchedMsg{results: Fetch(cfg, token, now), at: now}
	}
}

func (m *model) Init() tea.Cmd {
	return tea.Batch(m.spin.Tick, fetchCmd(m.cfg, m.token), clock())
}

// refresh recharge la config (thèmes et fréquence modifiés à chaud) puis relance la recherche.
func (m *model) refresh() tea.Cmd {
	if m.loading {
		return nil
	}
	if c, d, err := loadConfigWithInterval(m.cfgPath, m.intervalOverride); err == nil {
		m.cfg, m.interval = c, d
	} else {
		m.status = "config non rechargée (ancienne conservée) : " + err.Error()
	}
	m.loading = true
	return tea.Batch(m.spin.Tick, fetchCmd(m.cfg, m.token))
}

// visible renvoie les dépôts affichés dans l'onglet courant,
// après filtre texte, filtre « nouveautés » et tri.
func (m *model) visible() []Item {
	if m.tab >= len(m.results) {
		return nil
	}
	q := strings.ToLower(strings.TrimSpace(m.filter.Value()))
	var out []Item
	for _, it := range m.results[m.tab].Items {
		if m.newOnly && !it.New {
			continue
		}
		if q != "" && !matches(it, q) {
			continue
		}
		out = append(out, it)
	}
	sort.SliceStable(out, func(i, j int) bool {
		switch m.sort {
		case sortRecent:
			return out[i].CreatedAt.After(out[j].CreatedAt)
		case sortName:
			return strings.ToLower(out[i].FullName) < strings.ToLower(out[j].FullName)
		}
		return out[i].Stars > out[j].Stars
	})
	return out
}

func matches(it Item, q string) bool {
	hay := strings.ToLower(it.FullName + " " + it.Description + " " + it.Language + " " + strings.Join(it.Topics, " "))
	for _, word := range strings.Fields(q) {
		if !strings.Contains(hay, word) {
			return false
		}
	}
	return true
}

// resetView remet le curseur en haut après un changement de filtre ou de tri.
func (m *model) resetView() {
	for i := range m.cursor {
		m.cursor[i], m.offset[i] = 0, 0
	}
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

	case clockMsg:
		m.now = time.Time(msg)
		if !m.loading && !m.nextRun.IsZero() && !m.now.Before(m.nextRun) {
			return m, tea.Batch(m.refresh(), clock())
		}
		return m, clock()

	case spinner.TickMsg:
		if !m.loading {
			return m, nil // arrête l'animation entre deux recherches
		}
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd

	case fetchedMsg:
		m.results, m.lastRun, m.loading = msg.results, msg.at, false
		m.nextRun = msg.at.Add(m.interval)
		m.resetCursors()
		m.moveCursor(0)
		if _, err := saveReport(m.cfg, renderMarkdown(msg.results, msg.at, false), msg.at); err != nil {
			m.status = "rapport non enregistré : " + err.Error()
		}

	case statusMsg:
		m.status = string(msg)

	case tea.KeyMsg:
		m.status = ""
		if m.filtering {
			return m, m.updateFilter(msg)
		}
		if m.showHelp { // n'importe quelle touche ferme l'aide
			m.showHelp = false
			if msg.String() == "ctrl+c" {
				return m, tea.Quit
			}
			return m, nil
		}
		switch msg.String() {
		case "?":
			m.showHelp = true
		case "/":
			m.filtering = true
			return m, m.filter.Focus()
		case "esc":
			if m.filter.Value() != "" {
				m.filter.SetValue("")
				m.resetView()
			}
		case "s":
			m.sort = (m.sort + 1) % sortModes
			m.resetView()
			m.status = "tri : " + m.sort.String()
		case "n":
			m.newOnly = !m.newOnly
			m.resetView()
			if m.newOnly {
				m.status = "nouveautés uniquement"
			} else {
				m.status = "tous les dépôts"
			}
		case "q", "ctrl+c":
			return m, tea.Quit
		case "r":
			return m, m.refresh()
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

// updateFilter gère la saisie du filtre : le résultat se met à jour à chaque touche.
func (m *model) updateFilter(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "enter":
		m.filtering = false
		m.filter.Blur()
		return nil
	case "esc":
		m.filtering = false
		m.filter.Blur()
		m.filter.SetValue("")
		m.resetView()
		return nil
	case "ctrl+c":
		return tea.Quit
	case "up", "down":
		m.moveCursor(map[string]int{"up": -1, "down": 1}[msg.String()])
		return nil
	}
	before := m.filter.Value()
	var cmd tea.Cmd
	m.filter, cmd = m.filter.Update(msg)
	if m.filter.Value() != before {
		m.resetView()
	}
	return cmd
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
	if m.showHelp {
		body = lipgloss.NewStyle().MaxHeight(h).MaxWidth(m.width).Render(
			lipgloss.Place(m.width, h, lipgloss.Center, lipgloss.Center, m.viewHelp()))
	} else if m.width >= 100 { // assez large : liste à gauche, détail à droite
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
		now := m.now
		if now.IsZero() {
			now = time.Now()
		}
		countdown := lipgloss.NewStyle().Foreground(cAccent).Background(cBarBg).Render(humanDur(m.nextRun.Sub(now)))
		// Du plus détaillé au plus court : on garde la première version qui tient.
		for _, c := range []string{
			sBar.Render("vérifié à "+m.lastRun.Format("15:04")+"  ·  prochaine dans ") + countdown +
				sBar.Render("  ·  tous les "+humanDur(m.interval)),
			sBar.Render("vérifié à "+m.lastRun.Format("15:04")+"  ·  prochaine dans ") + countdown,
			sBar.Render("prochaine dans ") + countdown,
			countdown,
		} {
			right = c
			if lipgloss.Width(left)+lipgloss.Width(c)+1 <= m.width {
				break
			}
		}
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

	var flags []string
	if v := m.filter.Value(); v != "" && !m.filtering {
		flags = append(flags, sKey.Render("⌕")+sMuted.Render(" « "+v+" »"))
	}
	if m.newOnly {
		flags = append(flags, sTabBadge.Render("●")+sMuted.Render(" nouveautés"))
	}
	flags = append(flags, sKey.Render("⇅")+sMuted.Render(" "+m.sort.String()))
	right := strings.Join(flags, sRule.Render("  │  ")) + " "

	if gap := m.width - lipgloss.Width(line) - lipgloss.Width(right); gap >= 2 {
		return line + strings.Repeat(" ", gap) + right
	}
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
		switch {
		case m.filter.Value() != "":
			return "\n  " + sMuted.Render("Aucun résultat pour « "+m.filter.Value()+" ».  ") +
				sKey.Render("esc") + sMuted.Render(" pour effacer le filtre")
		case m.newOnly:
			return "\n  " + sMuted.Render("Rien de nouveau ici.  ") +
				sKey.Render("n") + sMuted.Render(" pour tout afficher")
		}
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
		return "" // rien à détailler : pas de cadre vide
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
	if m.filtering {
		m.filter.Width = max(m.width-40, 10)
		hint := sKey.Render("⏎") + sMuted.Render(" valider  ") + sKey.Render("esc") + sMuted.Render(" annuler ")
		return " " + m.filter.View() + strings.Repeat(" ", max(m.width-lipgloss.Width(m.filter.View())-lipgloss.Width(hint)-1, 1)) + hint
	}
	if m.status != "" {
		return ansi.Truncate(" "+sKey.Render("›")+" "+sDesc.Render(m.status), m.width, "…")
	}
	keys := []struct{ k, v string }{
		{"↑↓", "naviguer"}, {"←→", "veilles"}, {"⏎", "ouvrir"}, {"/", "filtrer"},
		{"s", "trier"}, {"n", "nouveautés"}, {"r", "actualiser"}, {"?", "aide"}, {"q", "quitter"},
	}
	if m.width < 125 { // terminal étroit : l'essentiel, le reste est dans l'aide
		keys = []struct{ k, v string }{
			{"↑↓", "naviguer"}, {"⏎", "ouvrir"}, {"/", "filtrer"}, {"?", "aide"}, {"q", "quitter"},
		}
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

func (m *model) viewHelp() string {
	groups := []struct {
		title string
		keys  [][2]string
	}{
		{"Navigation", [][2]string{
			{"↑ ↓  j k", "dépôt précédent / suivant"},
			{"← →  h l  tab", "veille précédente / suivante"},
			{"PgUp PgDn  espace", "page précédente / suivante"},
			{"g G  début fin", "premier / dernier dépôt"},
		}},
		{"Actions", [][2]string{
			{"⏎  o", "ouvrir le dépôt dans le navigateur"},
			{"r", "actualiser maintenant (recharge la config)"},
		}},
		{"Affichage", [][2]string{
			{"/", "filtrer (nom, description, langage, topic)"},
			{"esc", "effacer le filtre"},
			{"s", "trier : étoiles → plus récents → nom"},
			{"n", "nouveautés uniquement / tout"},
		}},
		{"", [][2]string{{"?", "aide"}, {"q  ctrl+c", "quitter"}}},
	}
	compact := m.listHeight() < 28 // petit terminal : pas de lignes vides entre groupes
	var b strings.Builder
	b.WriteString(sRepoSel.Render("Raccourcis clavier") + "\n")
	for _, g := range groups {
		if !compact {
			b.WriteString("\n")
		}
		if g.title != "" {
			b.WriteString(sMuted.Render(strings.ToUpper(g.title)) + "\n")
		}
		for _, k := range g.keys {
			b.WriteString(sKey.Render(fmt.Sprintf("%-20s", k[0])) + sDesc.Render(k[1]) + "\n")
		}
	}
	b.WriteString("\n" + sMuted.Render(fmt.Sprintf("Vérification tous les %s · %s", humanDur(m.interval), m.cfgPath)))
	b.WriteString("\n" + sMuted.Render("Une touche pour fermer"))
	pad := 1
	if compact {
		pad = 0
	}
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(cAccent).
		Padding(pad, 3).MaxHeight(m.listHeight()).Render(b.String())
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

// humanDur : 90s → « 1 min 30 s », 26h → « 1 j 2 h ».
func humanDur(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	d = d.Round(time.Second)
	days := int(d.Hours()) / 24
	h := int(d.Hours()) % 24
	mn := int(d.Minutes()) % 60
	sec := int(d.Seconds()) % 60
	switch {
	case days > 0 && h > 0:
		return fmt.Sprintf("%d j %d h", days, h)
	case days > 0:
		return fmt.Sprintf("%d j", days)
	case h > 0:
		return fmt.Sprintf("%d h %02d min", h, mn)
	case mn > 0:
		return fmt.Sprintf("%d min %02d s", mn, sec)
	}
	return fmt.Sprintf("%d s", sec)
}
