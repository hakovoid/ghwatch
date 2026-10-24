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
	sortTrend
	sortRecent
	sortName
	sortModes
)

func (s sortMode) String() string {
	return [...]string{"étoiles", "progression", "plus récents", "nom"}[s]
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
	fetched bool // au moins une veille a été vérifiée (sinon : lecture du cache)
}

// readmeWantMsg : le dépôt sélectionné n'a pas bougé depuis un instant, on
// peut aller chercher son README (évite une requête par dépôt survolé).
type readmeWantMsg struct {
	repo Repo
	seq  int
}

type readmeDoneMsg struct {
	name string
	err  error
}

type model struct {
	cfgPath, intervalOverride, token string
	cfg                              *Config
	interval                         time.Duration
	st                               *State // mémoire locale, partagée avec les tâches de fond

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

	sideBySide bool // détail à droite de la liste plutôt qu'en dessous
	split      int  // part de la liste en % (hauteur, ou largeur côte à côte)

	selName       string          // dépôt sélectionné au dernier rendu
	detailOff     int             // défilement du panneau de détail
	detailLines   int             // lignes du détail au dernier rendu
	readmeSeq     int             // invalide les demandes de README périmées
	readmePending map[string]bool // README en cours de chargement
	readmeErr     map[string]string

	search        *Result // onglet de recherche ponctuelle (nil = fermé)
	searchLoading bool
	searchMode    bool // saisie de la recherche en cours
	searchInput   textinput.Model
	history       []string // recherches passées, la plus récente en premier
	histIdx       int      // -1 = saisie en cours, sinon index dans history
	histDraft     string   // saisie en cours, retrouvée en redescendant
}

func runTUI(cfgPath, intervalOverride, token string) error {
	cfg, interval, err := loadConfigWithInterval(cfgPath, intervalOverride)
	if err != nil {
		return err
	}
	log.SetOutput(io.Discard) // les logs casseraient l'affichage

	m := newModel(cfg, interval)
	m.cfgPath, m.intervalOverride, m.token = cfgPath, intervalOverride, token
	m.loadCached(time.Now())
	if token == "" {
		m.status = "sans GITHUB_TOKEN : 10 recherches/min, vérifications plus lentes"
	}
	_, err = tea.NewProgram(m, tea.WithAltScreen()).Run()
	return err
}

func newModel(cfg *Config, interval time.Duration) *model {
	sp := spinner.New()
	sp.Spinner = spinner.MiniDot
	sp.Style = lipgloss.NewStyle().Foreground(cAccent)
	m := &model{
		cfg: cfg, interval: interval, spin: sp,
		filter: newFilterInput(), searchInput: newSearchInput(),
		split: splitDefault, st: loadState(cfg.StateFile),
		readmePending: map[string]bool{}, readmeErr: map[string]string{},
	}
	m.resetCursors()
	return m
}

// loadCached affiche tout de suite les résultats mémorisés ; GitHub n'est
// interrogé que pour les veilles jamais vérifiées ou dont l'intervalle est écoulé.
func (m *model) loadCached(now time.Time) {
	m.results, _ = m.st.check(m.cfg, m.token, now, func(Watch) bool { return false })
	m.lastRun, m.nextRun = m.st.schedule(m.cfg, m.interval, now)
	m.loading = !now.Before(m.nextRun)
	m.resetCursors()
}

func (m *model) resetCursors() {
	n := m.watchTabs() + markTabs
	if m.search != nil {
		n++
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

// fetchCmd vérifie les veilles dont l'intervalle est écoulé (toutes si force).
func fetchCmd(cfg *Config, st *State, token string, interval time.Duration, force bool) tea.Cmd {
	return func() tea.Msg {
		now := time.Now()
		var due func(Watch) bool
		if !force {
			due = func(w Watch) bool { return st.due(w, interval, now) }
		}
		results, fetched := st.check(cfg, token, now, due)
		if fetched {
			_ = st.save(cfg.StateFile)
		}
		return fetchedMsg{results: results, at: now, fetched: fetched}
	}
}

func (m *model) Init() tea.Cmd {
	if !m.loading {
		return clock()
	}
	return tea.Batch(m.spin.Tick, fetchCmd(m.cfg, m.st, m.token, m.interval, false), clock())
}

// refresh recharge la config (thèmes et fréquence modifiés à chaud) puis
// vérifie les veilles dues, ou toutes si force (touche r).
func (m *model) refresh(force bool) tea.Cmd {
	if m.loading {
		return nil
	}
	if c, d, err := loadConfigWithInterval(m.cfgPath, m.intervalOverride); err == nil {
		m.cfg, m.interval = c, d
	} else {
		m.status = "config non rechargée (ancienne conservée) : " + err.Error()
	}
	m.loading = true
	return tea.Batch(m.spin.Tick, fetchCmd(m.cfg, m.st, m.token, m.interval, force))
}

// visible renvoie les dépôts affichés dans l'onglet courant,
// après filtre texte, filtre « nouveautés » et tri.
func (m *model) visible() []Item {
	tabs := m.tabs()
	if m.tab >= len(tabs) {
		return nil
	}
	q := strings.ToLower(strings.TrimSpace(m.filter.Value()))
	var out []Item
	for _, it := range tabs[m.tab].Items {
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
		case sortTrend:
			gi, gj := out[i].Trend.Gain, out[j].Trend.Gain
			if !out[i].Trend.OK {
				gi = -1 << 30 // progression inconnue : en fin de liste
			}
			if !out[j].Trend.OK {
				gj = -1 << 30
			}
			if gi != gj {
				return gi > gj
			}
			return out[i].Stars > out[j].Stars
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

// Partage de l'écran entre la liste et le détail (touches + - =).
const (
	splitDefault = 55
	splitMin     = 20
	splitMax     = 80
	splitStep    = 10
)

// bodyHeight : hauteur disponible sous les onglets.
func (m *model) bodyHeight() int {
	return max(m.height-5, itemHeight) // barre de titre, onglets, règle, pied
}

// showSideBySide : détail à droite, si demandé et si le terminal est assez large.
func (m *model) showSideBySide() bool { return m.sideBySide && m.width >= 100 }

// showStacked : détail sous la liste, si le terminal est assez haut pour les deux.
func (m *model) showStacked() bool { return !m.showSideBySide() && m.bodyHeight() >= 14 }

func (m *model) listHeight() int {
	h := m.bodyHeight()
	if m.showStacked() {
		return max(h*m.split/100, itemHeight)
	}
	return h
}

// resize agrandit (delta > 0) ou réduit la liste, le détail prenant le reste.
func (m *model) resize(delta int) {
	m.split = min(max(m.split+delta, splitMin), splitMax)
	m.moveCursor(0) // le nombre de dépôts par page a changé
	m.status = fmt.Sprintf("liste %d %% · détail %d %%", m.split, 100-m.split)
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
	cmd := m.update(msg)
	return m, tea.Batch(cmd, m.followSelection())
}

// followSelection remet le détail en haut quand la sélection change et
// demande le README du nouveau dépôt s'il n'est pas déjà en mémoire.
func (m *model) followSelection() tea.Cmd {
	it, ok := m.selected()
	if !ok || it.FullName == m.selName {
		if !ok {
			m.selName = ""
		}
		return nil
	}
	m.selName, m.detailOff = it.FullName, 0
	if _, fresh := m.st.readme(it.FullName, time.Now()); fresh || m.readmePending[it.FullName] {
		return nil
	}
	m.readmeSeq++
	want := readmeWantMsg{repo: it.Repo, seq: m.readmeSeq}
	return tea.Tick(400*time.Millisecond, func(time.Time) tea.Msg { return want })
}

func fetchReadmeCmd(st *State, statePath string, r Repo, token string) tea.Cmd {
	return func() tea.Msg {
		text, err := fetchReadme(r.FullName, token)
		if err == nil {
			st.setReadme(r, text, time.Now())
			_ = st.save(statePath)
		}
		return readmeDoneMsg{name: r.FullName, err: err}
	}
}

// saveCmd enregistre l'état en tâche de fond (après un marque-page).
func (m *model) saveCmd() tea.Cmd {
	st, path := m.st, m.cfg.StateFile
	return func() tea.Msg {
		if err := st.save(path); err != nil {
			return statusMsg("état non enregistré : " + err.Error())
		}
		return nil
	}
}

// toggleMark pose ou retire un marque-page sur le dépôt sélectionné.
func (m *model) toggleMark(k mark) tea.Cmd {
	it, ok := m.selected()
	if !ok {
		return nil
	}
	on := m.st.toggle(it.Repo, k)
	label := [...]string{"favoris", "à voir"}[k]
	if on {
		m.status = it.FullName + " ajouté à « " + label + " »"
	} else {
		m.status = it.FullName + " retiré de « " + label + " »"
	}
	m.moveCursor(0) // l'onglet Favoris / À voir a pu rétrécir
	return m.saveCmd()
}

func (m *model) update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.moveCursor(0)

	case clockMsg:
		m.now = time.Time(msg)
		if !m.loading && !m.nextRun.IsZero() && !m.now.Before(m.nextRun) {
			return tea.Batch(m.refresh(false), clock())
		}
		return clock()

	case spinner.TickMsg:
		if !m.loading && !m.searchLoading {
			return nil
		}
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return cmd

	case fetchedMsg:
		m.results, m.loading = msg.results, false
		m.lastRun, m.nextRun = m.st.schedule(m.cfg, m.interval, msg.at)
		m.resetCursors()
		m.moveCursor(0)
		if !msg.fetched {
			break
		}
		if _, err := saveReport(m.cfg, renderMarkdown(msg.results, msg.at, false), msg.at); err != nil {
			m.status = "rapport non enregistré : " + err.Error()
		}

	case readmeWantMsg:
		if msg.seq == m.readmeSeq && msg.repo.FullName == m.selName && !m.readmePending[m.selName] {
			m.readmePending[m.selName] = true
			delete(m.readmeErr, m.selName)
			return fetchReadmeCmd(m.st, m.cfg.StateFile, msg.repo, m.token)
		}

	case readmeDoneMsg:
		delete(m.readmePending, msg.name)
		if msg.err != nil {
			m.readmeErr[msg.name] = msg.err.Error()
		}

	case searchDoneMsg:
		if m.search != nil && m.search.Query == msg.result.Query {
			r := msg.result
			m.search, m.searchLoading = &r, false
			if r.Err == nil {
				m.status = fmt.Sprintf("%d dépôt(s) trouvé(s) · w pour en faire une veille · x pour fermer", len(r.Items))
			}
		}

	case statusMsg:
		m.status = string(msg)

	case tea.KeyMsg:
		m.status = ""
		if m.filtering {
			return m.updateFilter(msg)
		}
		if m.searchMode {
			return m.updateSearchInput(msg)
		}
		if m.showHelp { // n'importe quelle touche ferme l'aide
			m.showHelp = false
			if msg.String() == "ctrl+c" {
				return tea.Quit
			}
			return nil
		}
		switch msg.String() {
		case "?":
			m.showHelp = true
		case "S":
			m.searchMode = true
			m.history, m.histIdx, m.histDraft = loadHistory(historyPath(m.cfg)), -1, ""
			if m.search != nil {
				m.searchInput.SetValue(m.search.Query)
				m.searchInput.CursorEnd()
			}
			return m.searchInput.Focus()
		case "x":
			if m.onSearchTab() {
				m.closeSearch()
			}
		case "w":
			if m.onSearchTab() {
				m.saveSearchAsWatch()
			}
		case "/":
			m.filtering = true
			return m.filter.Focus()
		case "esc":
			if m.filter.Value() != "" {
				m.filter.SetValue("")
				m.resetView()
			}
		case "s":
			m.sort = (m.sort + 1) % sortModes
			m.resetView()
			m.status = "tri : " + m.sort.String()
			if m.sort == sortTrend {
				m.status += " (étoiles gagnées sur " + spanLabel(m.cfg.trendWindow()) + ")"
			}
		case "n":
			m.newOnly = !m.newOnly
			m.resetView()
			if m.newOnly {
				m.status = "nouveautés uniquement"
			} else {
				m.status = "tous les dépôts"
			}
		case "v":
			m.sideBySide = !m.sideBySide
			m.moveCursor(0)
			switch {
			case m.sideBySide && !m.showSideBySide():
				m.status = "vue côte à côte : terminal trop étroit (100 colonnes minimum)"
			case m.sideBySide:
				m.status = "vue : détail à droite"
			default:
				m.status = "vue : détail en dessous"
			}
		case "+":
			m.resize(splitStep)
		case "-":
			m.resize(-splitStep)
		case "=":
			m.split = splitDefault
			m.resize(0)
		case "q", "ctrl+c":
			return tea.Quit
		case "r":
			return m.refresh(true)
		case "f":
			return m.toggleMark(markFav)
		case "a":
			return m.toggleMark(markLater)
		case "J":
			m.detailOff++
		case "K":
			m.detailOff = max(m.detailOff-1, 0)
		case "c":
			if it, ok := m.selected(); ok {
				url := it.HTMLURL
				// en tâche de fond : un outil de presse-papiers lent ne fige pas l'écran
				return func() tea.Msg {
					msg := "lien copié : " + url
					if copyToClipboard(url) == "terminal" {
						msg += " (via le terminal)"
					}
					return statusMsg(msg)
				}
			}
		case "enter", "o":
			if it, ok := m.selected(); ok {
				url := it.HTMLURL
				return func() tea.Msg {
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
	return nil
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
	h := m.bodyHeight()
	var body string
	switch {
	case m.showHelp:
		body = lipgloss.NewStyle().MaxHeight(h).MaxWidth(m.width).Render(
			lipgloss.Place(m.width, h, lipgloss.Center, lipgloss.Center, m.viewHelp()))
	case m.showSideBySide(): // liste à gauche, détail à droite
		lw := m.width * m.split / 100
		list := lipgloss.NewStyle().Width(lw).Height(h).MaxHeight(h).Render(m.viewList(lw - 1))
		body = lipgloss.JoinHorizontal(lipgloss.Top, list, m.viewDetail(m.width-lw, h))
	case m.showStacked(): // liste en haut, détail en dessous
		lh := m.listHeight()
		list := lipgloss.NewStyle().Width(m.width).Height(lh).MaxHeight(lh).Render(m.viewList(m.width))
		body = lipgloss.NewStyle().Height(h).MaxHeight(h).Render(
			lipgloss.JoinVertical(lipgloss.Left, list, m.viewDetail(m.width, h-lh)))
	default: // terminal trop petit : la liste seule
		body = lipgloss.NewStyle().Height(h).MaxHeight(h).Render(m.viewList(m.width))
	}
	return lipgloss.JoinVertical(lipgloss.Left, header, tabs, rule, body, footer)
}

func (m *model) viewHeader() string {
	left := sLogo.Render("◆ ghwatch") + sBar.Render("  veille GitHub ")
	var right string
	switch {
	case m.loading:
		right = m.spin.View() + " vérification des veilles…"
	case m.searchLoading:
		right = m.spin.View() + " recherche en cours…"
	case !m.lastRun.IsZero():
		now := m.now
		if now.IsZero() {
			now = time.Now()
		}
		when := "vérifié à " + m.lastRun.Format("15:04")
		if m.lastRun.Format("2006-01-02") != now.Format("2006-01-02") {
			when = "vérifié le " + m.lastRun.Format("02/01 à 15:04")
		}
		countdown := lipgloss.NewStyle().Foreground(cAccent).Background(cBarBg).Render(humanDur(m.nextRun.Sub(now)))
		// Du plus détaillé au plus court : on garde la première version qui tient.
		for _, c := range []string{
			sBar.Render(when+"  ·  prochaine dans ") + countdown +
				sBar.Render("  ·  tous les "+humanDur(m.interval)),
			sBar.Render(when+"  ·  prochaine dans ") + countdown,
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
	all := m.tabs()
	right := m.viewTabFlags(all)
	// Noms de veilles raccourcis si nécessaire, pour que tous les onglets
	// (Favoris et À voir compris) restent visibles.
	var line, alone string
	for _, maxName := range []int{0, 32, 28, 24, 21, 18, 16, 14, 12, 10, 8, 6} {
		line = m.renderTabs(all, maxName)
		if lipgloss.Width(line)+lipgloss.Width(right)+2 <= m.width {
			return line + strings.Repeat(" ", m.width-lipgloss.Width(line)-lipgloss.Width(right)) + right
		}
		if alone == "" && lipgloss.Width(line) <= m.width {
			alone = line // tient sans les indicateurs de droite
		}
	}
	if alone != "" {
		return alone
	}
	return ansi.Truncate(line, m.width, "…")
}

// renderTabs dessine la barre d'onglets, noms de veilles limités à maxName
// caractères (0 = complets ; l'onglet actif garde au moins 24 caractères).
func (m *model) renderTabs(all []Result, maxName int) string {
	var tabs []string
	for i, w := range m.cfg.Watches[:min(len(m.cfg.Watches), m.watchTabs())] {
		label := w.Name
		if i < len(m.results) {
			label = m.results[i].Watch.Name
		}
		if maxName > 0 {
			limit := maxName
			if i == m.tab {
				limit = max(maxName, 24)
			}
			if _, tail, ok := cutLast(label, " · "); ok && lipgloss.Width(label) > limit {
				label = tail // « IA & agents · ce qui monte » → « ce qui monte »
			}
			label = ansi.Truncate(label, limit, "…")
		}
		if i < len(m.results) {
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
	for _, k := range []mark{markFav, markLater} {
		i := m.markTab(k)
		label := all[i].Watch.Name
		if n := len(all[i].Items); n > 0 {
			label += " " + fmt.Sprint(n)
		}
		if i == m.tab {
			tabs = append(tabs, sTabActive.Background(cMark).Render(label))
		} else {
			tabs = append(tabs, sTabInactive.Foreground(cMark).Render(label))
		}
	}
	if m.search != nil {
		label := "⌕ " + ansi.Truncate(m.search.Query, 24, "…")
		if m.onSearchTab() {
			tabs = append(tabs, sTabActive.Background(cAccent2).Render(label))
		} else {
			tabs = append(tabs, sTabInactive.Foreground(cAccent2).Render(label))
		}
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, tabs...)
}

func cutLast(s, sep string) (before, after string, found bool) {
	if i := strings.LastIndex(s, sep); i >= 0 {
		return s[:i], s[i+len(sep):], true
	}
	return s, "", false
}

// viewTabFlags : état de l'onglet courant (erreurs, filtre, tri), à droite des onglets.
func (m *model) viewTabFlags(all []Result) string {
	var flags []string
	if m.tab < len(all) {
		switch r := all[m.tab]; {
		case r.Err != nil && len(r.Items) > 0:
			flags = append(flags, sErr.Render("⚠ vérification échouée, résultats mémorisés"))
		case r.Failed > 0:
			flags = append(flags, sErr.Render(fmt.Sprintf("⚠ %d requête(s) en échec", r.Failed)))
		}
	}
	if v := m.filter.Value(); v != "" && !m.filtering {
		flags = append(flags, sKey.Render("⌕")+sMuted.Render(" « "+v+" »"))
	}
	if m.newOnly {
		flags = append(flags, sTabBadge.Render("●")+sMuted.Render(" nouveautés"))
	}
	sortLabel := m.sort.String()
	if m.sort == sortTrend {
		sortLabel += " sur " + spanLabel(m.cfg.trendWindow())
	}
	flags = append(flags, sKey.Render("⇅")+sMuted.Render(" "+sortLabel))
	return strings.Join(flags, sRule.Render("  │  ")) + " "
}

func (m *model) viewList(width int) string {
	if m.onSearchTab() && m.searchLoading {
		return "\n  " + m.spin.View() + sMuted.Render(" Recherche « "+m.search.Query+" » sur GitHub…")
	}
	items := m.visible()
	if tabs := m.tabs(); m.tab < len(tabs) && len(items) == 0 {
		if m.loading && m.tab < m.watchTabs() && tabs[m.tab].CheckedAt.IsZero() {
			return "\n  " + m.spin.View() + sMuted.Render(" Interrogation de GitHub…")
		}
		if err := tabs[m.tab].Err; err != nil {
			msg := ansi.Wordwrap(err.Error(), width-6, " ")
			return "\n  " + sErr.Render("⚠  Erreur") + "\n\n" +
				lipgloss.NewStyle().PaddingLeft(2).Foreground(cMuted).Render(msg)
		}
	}
	if len(items) == 0 {
		switch {
		case m.filter.Value() != "":
			return "\n  " + sMuted.Render("Aucun résultat pour « "+m.filter.Value()+" ».  ") +
				sKey.Render("esc") + sMuted.Render(" pour effacer le filtre")
		case m.newOnly:
			return "\n  " + sMuted.Render("Rien de nouveau ici.  ") +
				sKey.Render("n") + sMuted.Render(" pour tout afficher")
		}
		switch m.tab {
		case m.markTab(markFav):
			return "\n  " + sMuted.Render("Aucun favori.  ") + sKey.Render("f") + sMuted.Render(" sur un dépôt pour l'ajouter")
		case m.markTab(markLater):
			return "\n  " + sMuted.Render("Rien à voir pour l'instant.  ") + sKey.Render("a") + sMuted.Render(" sur un dépôt pour l'ajouter")
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

// Colonne de droite de la liste (étoiles, progression) : la description
// s'arrête avant, pour que les chiffres restent alignés et lisibles.
const statsCol = 11

func (m *model) viewItem(it Item, selected bool, width int) string {
	cursor := "  "
	repoStyle, descStyle := sRepo, sMuted
	if selected {
		cursor = sCursor.Render("▌ ")
		repoStyle, descStyle = sRepoSel, sDesc
	}
	left := max(width-statsCol-1, 10) - lipgloss.Width(cursor)

	owner, name, _ := strings.Cut(it.FullName, "/")
	title := hyperlink(it.HTMLURL, sOwner.Render(owner+"/")+repoStyle.Render(name))
	var badges []string
	if it.New {
		badges = append(badges, sNewBadge.Render("NEW"))
	}
	if fav, later := m.st.marks(it.FullName); fav || later {
		var mk string
		if fav {
			mk += "♥"
		}
		if later {
			mk += "◷"
		}
		badges = append(badges, sMark.Render(mk))
	}
	if it.Archived {
		badges = append(badges, sMuted.Render("archivé"))
	}
	if len(badges) > 0 {
		title += " " + strings.Join(badges, " ")
	}

	desc := strings.Join(strings.Fields(it.Description), " ")
	if desc == "" {
		desc = "pas de description"
	}
	meta := langBadge(it.Language) + sMuted.Render(" · "+relTime(it.CreatedAt)+" · ")

	stars := sStars.Render("★ " + humanStars(it.Stars))
	var gain string
	if tr := it.Trend; tr.OK && tr.Gain > 0 {
		gain = sUp.Render("▲ " + humanGain(tr.Gain))
	}
	col := lipgloss.NewStyle().Width(statsCol).Align(lipgloss.Right)

	line1 := lipgloss.NewStyle().Width(left).Render(ansi.Truncate(title, left, "…"))
	line2 := lipgloss.NewStyle().Width(left).Render(ansi.Truncate(meta+descStyle.Render(desc), left, "…"))
	return cursor + line1 + " " + col.Render(stars) + "\n" +
		cursor + line2 + " " + col.Render(gain)
}

func (m *model) viewDetail(width, height int) string {
	box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(cFaint).
		Padding(0, 2).Width(width - 2).Height(height - 2).MaxHeight(height)
	it, ok := m.selected()
	if !ok {
		return "" // rien à détailler : pas de cadre vide
	}
	inner := width - 6
	// Large : texte (description, README) à gauche, fiche à droite.
	textW, infoW := inner, inner
	twoCols := inner >= 90
	if twoCols {
		infoW = 40
		textW = inner - infoW - 4
	}

	owner, name, _ := strings.Cut(it.FullName, "/")
	title := hyperlink(it.HTMLURL, sOwner.Render(owner+"/")+sRepoSel.Render(name))
	if it.New {
		title += " " + sNewBadge.Render("NEW")
	}
	fav, later := m.st.marks(it.FullName)
	marks := sMuted.Render("♡ favori (f)   ◷ à voir (a)")
	switch {
	case fav && later:
		marks = sMark.Render("♥ favori   ◷ à voir")
	case fav:
		marks = sMark.Render("♥ favori") + sMuted.Render("   ◷ à voir (a)")
	case later:
		marks = sMuted.Render("♡ favori (f)   ") + sMark.Render("◷ à voir")
	}
	title += "   " + marks

	desc := strings.TrimSpace(it.Description)
	if desc == "" {
		desc = sMuted.Render("Pas de description.")
	}
	desc = sDesc.Render(ansi.Wordwrap(desc, textW, " "))

	stats := sStars.Render("★ "+humanStars(it.Stars)) + sMuted.Render("   ⑂ "+humanStars(it.Forks)+
		"   ◎ "+humanStars(it.Issues)+" issues")

	license := "—"
	if it.License != nil && it.License.SPDX != "" && it.License.SPDX != "NOASSERTION" {
		license = it.License.SPDX
	}
	row := func(k, v string) string {
		return sMuted.Render(fmt.Sprintf("%-15s", k)) + v
	}
	facts := []string{
		row("Langage", langBadge(it.Language)),
		row("Licence", sDesc.Render(license)),
		row("Créé", sDesc.Render(it.CreatedAt.Format("02/01/2006"))+sMuted.Render("  ("+relTime(it.CreatedAt)+")")),
		row("Dernier commit", sDesc.Render(relTime(it.PushedAt))),
	}
	if !it.Seen.IsZero() {
		facts = append(facts, row("Repéré", sDesc.Render(it.Seen.Format("02/01/2006"))))
	}
	if hp := strings.TrimSpace(it.Homepage); hp != "" {
		facts = append(facts, row("Site", hyperlink(hp, lipgloss.NewStyle().Foreground(cAccent2).
			Render(ansi.Truncate(hp, max(infoW-15, 10), "…")))))
	}
	if it.Archived {
		facts = append(facts, row("État", sErr.Render("archivé (plus maintenu)")))
	}

	info := []string{stats, m.viewTrend(it, infoW), "", strings.Join(facts, "\n")}

	if len(it.Topics) > 0 {
		var chips []string
		lineW := 0
		var lines []string
		for _, t := range it.Topics {
			c := sTopic.Render(t)
			if lineW > 0 && lineW+lipgloss.Width(c)+1 > infoW {
				lines = append(lines, strings.Join(chips, " "))
				chips, lineW = nil, 0
			}
			chips = append(chips, c)
			lineW += lipgloss.Width(c) + 1
		}
		lines = append(lines, strings.Join(chips, " "))
		info = append(info, "", strings.Join(lines, "\n"))
	}

	link := hyperlink(it.HTMLURL, lipgloss.NewStyle().Foreground(cAccent2).Underline(true).
		Render(ansi.Truncate(it.HTMLURL, infoW, "…")))
	info = append(info, "", link, sMuted.Render(ansi.Wordwrap("clic ou ⏎ pour ouvrir · c pour copier le lien", infoW, " ")))

	text := []string{desc, "", m.viewReadme(it, textW)}
	var parts []string
	if twoCols {
		parts = []string{title, "", lipgloss.JoinHorizontal(lipgloss.Top,
			lipgloss.NewStyle().Width(textW).Render(strings.Join(text, "\n")),
			lipgloss.NewStyle().Width(4).Render(""),
			lipgloss.NewStyle().Width(infoW).Render(strings.Join(info, "\n")))}
	} else {
		parts = append(append([]string{title, "", desc, ""}, info...), "", m.viewReadme(it, textW))
	}

	// défilement (J / K) : on coupe le contenu plutôt que le cadre
	lines := strings.Split(strings.Join(parts, "\n"), "\n")
	room := max(height-2, 1)
	m.detailLines = len(lines)
	m.detailOff = min(m.detailOff, max(len(lines)-room+1, 0))
	lines = lines[min(m.detailOff, len(lines)):]
	if len(lines) > room {
		more := fmt.Sprintf("↓ J / K pour faire défiler (%d lignes de plus)", len(lines)-room+1)
		if m.showStacked() {
			more += " · - agrandit le détail"
		}
		lines = append(lines[:room-1], sMuted.Render(ansi.Truncate(more, inner, "…")))
	}
	return box.Render(strings.Join(lines, "\n"))
}

// viewReadme : début du README, chargé à la demande puis mémorisé.
func (m *model) viewReadme(it Item, width int) string {
	head := sMuted.Render("README")
	text, _ := m.st.readme(it.FullName, time.Now())
	switch {
	case text != "":
		return head + "\n" + sDesc.Render(ansi.Wordwrap(text, width, " "))
	case m.readmePending[it.FullName]:
		return head + "\n" + sMuted.Render("chargement…")
	case m.readmeErr[it.FullName] != "":
		return head + "\n" + sMuted.Render(ansi.Wordwrap("indisponible : "+m.readmeErr[it.FullName], width, " "))
	}
	if _, fetched := m.st.readme(it.FullName, time.Now()); fetched {
		return head + "\n" + sMuted.Render("Ce dépôt n'a pas de README.")
	}
	return head + "\n" + sMuted.Render("chargement…")
}

// viewTrend : progression des étoiles et mini-graphique, pour le panneau de détail.
func (m *model) viewTrend(it Item, width int) string {
	tr := it.Trend
	if !tr.OK {
		return sMuted.Render("Progression mesurée à partir de la prochaine vérification")
	}
	var head string
	switch {
	case tr.Gain > 0:
		head = sUp.Render("▲ "+humanGain(tr.Gain)+" ★") + sMuted.Render(" en "+spanLabel(tr.Span))
	case tr.Gain < 0:
		head = sErr.Render("▼ "+humanGain(tr.Gain)+" ★") + sMuted.Render(" en "+spanLabel(tr.Span))
	default:
		head = sMuted.Render("stable sur " + spanLabel(tr.Span))
	}
	if spk := sparkline(tr.Points, min(width-2, 40)); spk != "" {
		head += "\n" + sSpk.Render(spk) + sMuted.Render(fmt.Sprintf("  %d relevés", len(tr.Points)))
	}
	return head
}

func (m *model) viewFooter() string {
	if m.searchMode {
		hint := sKey.Render("⏎") + sMuted.Render(" chercher  ") + sKey.Render("esc") + sMuted.Render(" annuler ")
		if len(m.history) > 0 {
			pos := ""
			if m.histIdx >= 0 {
				pos = fmt.Sprintf(" %d/%d", m.histIdx+1, len(m.history))
			}
			hint = sKey.Render("↑↓") + sMuted.Render(" historique"+pos+"  ") + hint
		}
		// la saisie prend toute la place que laissent les indications
		m.searchInput.Width = max(m.width-lipgloss.Width(hint)-lipgloss.Width(m.searchInput.Prompt)-4, 10)
		in := m.searchInput.View()
		line := " " + in + strings.Repeat(" ", max(m.width-lipgloss.Width(in)-lipgloss.Width(hint)-1, 1)) + hint
		return ansi.Truncate(line, m.width, "")
	}
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
		{"f", "favori"}, {"a", "à voir"}, {"S", "chercher"}, {"s", "trier"}, {"+-", "taille"}, {"r", "actualiser"}, {"?", "aide"}, {"q", "quitter"},
	}
	if m.width < 140 { // terminal étroit : l'essentiel, le reste est dans l'aide
		keys = []struct{ k, v string }{
			{"↑↓", "naviguer"}, {"⏎", "ouvrir"}, {"f", "favori"}, {"a", "à voir"}, {"?", "aide"}, {"q", "quitter"},
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
			{"⏎  o  clic", "ouvrir le dépôt dans le navigateur"},
			{"c", "copier le lien du dépôt"},
			{"f", "ajouter / retirer des favoris  ♥"},
			{"a", "ajouter / retirer de « à voir »  ◷"},
			{"r", "tout revérifier maintenant (recharge la config)"},
		}},
		{"Recherche GitHub", [][2]string{
			{"S", "nouvelle recherche (syntaxe GitHub)"},
			{"↑ ↓  (en saisie)", "parcourir l'historique des recherches"},
			{"w", "enregistrer la recherche comme veille"},
			{"x", "fermer l'onglet de recherche"},
		}},
		{"Affichage", [][2]string{
			{"/", "filtrer (nom, description, langage, topic)"},
			{"esc", "effacer le filtre"},
			{"s", "trier : étoiles → progression → récents → nom"},
			{"n", "nouveautés uniquement / tout"},
		}},
		{"Disposition", [][2]string{
			{"v", "détail en dessous / à droite"},
			{"+  -", "agrandir / réduire la liste"},
			{"=", "partage par défaut"},
			{"J  K", "faire défiler le détail"},
		}},
		{"", [][2]string{{"?", "aide"}, {"q  ctrl+c", "quitter"}}},
	}
	compact := m.bodyHeight() < 28 // petit terminal : pas de lignes vides entre groupes
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
		Padding(pad, 3).MaxHeight(m.bodyHeight()).Render(b.String())
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
