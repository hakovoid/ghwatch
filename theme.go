package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// Palette : s'adapte automatiquement aux terminaux clairs et sombres.
// cAccent, cAccent2, cSelBg et cBarBg changent avec le thème (applyTheme).
var (
	cAccent  = lipgloss.AdaptiveColor{Light: "#5A3FD9", Dark: "#9D7CFF"}
	cAccent2 = lipgloss.AdaptiveColor{Light: "#0E8A86", Dark: "#3EE0C9"}
	cNew     = lipgloss.AdaptiveColor{Light: "#D6246E", Dark: "#FF5FA2"}
	cStar    = lipgloss.AdaptiveColor{Light: "#B7791F", Dark: "#F7C948"}
	cText    = lipgloss.AdaptiveColor{Light: "#1F1D2B", Dark: "#E6E3F2"}
	cMuted   = lipgloss.AdaptiveColor{Light: "#6B6880", Dark: "#8A86A3"}
	cFaint   = lipgloss.AdaptiveColor{Light: "#C9C6D9", Dark: "#3A3650"}
	cBarBg   = lipgloss.AdaptiveColor{Light: "#EEEBFA", Dark: "#1E1B2E"}
	cSelBg   = lipgloss.AdaptiveColor{Light: "#F3F0FF", Dark: "#262140"}
	cErr     = lipgloss.AdaptiveColor{Light: "#C53030", Dark: "#FF6B6B"}
	cMark    = lipgloss.AdaptiveColor{Light: "#C2410C", Dark: "#FB923C"} // favoris, à voir
	cUp      = lipgloss.AdaptiveColor{Light: "#178A3A", Dark: "#4ADE80"}
	cOnColor = lipgloss.Color("#FFFFFF")
)

// theme : une déclinaison de couleurs (mêmes noms que vulnkb), pour terminal
// sombre et clair : accent (logo, onglet actif, sélection), accent secondaire
// (touches, liens), fond de la ligne sélectionnée, fond des barres.
type theme struct {
	name                        string
	accent, accent2, sel, barBg lipgloss.AdaptiveColor
}

func ac(light, dark string) lipgloss.AdaptiveColor {
	return lipgloss.AdaptiveColor{Light: light, Dark: dark}
}

// themes, dans l'ordre où « T » les fait défiler.
var themes = []theme{
	{"violet", ac("#5A3FD9", "#9D7CFF"), ac("#0E8A86", "#3EE0C9"), ac("#F3F0FF", "#262140"), ac("#EEEBFA", "#1E1B2E")},
	{"bleu", ac("#0969DA", "#58A6FF"), ac("#1A7F37", "#3FB950"), ac("#DDF4FF", "#1B2A3D"), ac("#F0F6FC", "#161B22")},
	{"rose", ac("#BF3989", "#FF7EB6"), ac("#0969DA", "#79C0FF"), ac("#FFEFF7", "#3A1D2E"), ac("#FFF5FA", "#24161E")},
	{"vert", ac("#1A7F37", "#3FB950"), ac("#9A6700", "#D29922"), ac("#DAFBE1", "#16301F"), ac("#F0FBF3", "#121D16")},
	{"cyan", ac("#1B7C83", "#39C5CF"), ac("#8250DF", "#BC8CFF"), ac("#D6F5F7", "#133238"), ac("#EFFAFB", "#0F1F22")},
	{"orange", ac("#BC4C00", "#F0883E"), ac("#0969DA", "#58A6FF"), ac("#FFF1E5", "#3A2616"), ac("#FFF8F2", "#241A12")},
}

func themeNames() []string {
	var out []string
	for _, t := range themes {
		out = append(out, t.name)
	}
	return out
}

func validTheme(name string) bool {
	for _, t := range themes {
		if strings.EqualFold(t.name, name) {
			return true
		}
	}
	return false
}

// nextTheme renvoie le thème qui suit name (en boucle).
func nextTheme(name string) string {
	for i, t := range themes {
		if strings.EqualFold(t.name, name) {
			return themes[(i+1)%len(themes)].name
		}
	}
	return themes[0].name
}

// applyTheme installe le thème name (le premier si inconnu) et renvoie son nom.
func applyTheme(name string) string {
	t := themes[0]
	for _, x := range themes {
		if strings.EqualFold(x.name, name) {
			t = x
		}
	}
	cAccent, cAccent2, cSelBg, cBarBg = t.accent, t.accent2, t.sel, t.barBg
	buildStyles()
	return t.name
}

var (
	sLogo, sBar, sBarKey                                          lipgloss.Style
	sTabActive, sTabInactive, sTabBadge, sRule                    lipgloss.Style
	sOwner, sRepo, sRepoSel, sCursor, sNewBadge, sStars           lipgloss.Style
	sMuted, sDesc, sErr, sKey, sTopic, sMark, sSection, sUp, sSpk lipgloss.Style
)

func buildStyles() {
	sLogo = lipgloss.NewStyle().Bold(true).Foreground(cOnColor).Background(cAccent).Padding(0, 1)
	sBar = lipgloss.NewStyle().Background(cBarBg).Foreground(cMuted)
	sBarKey = lipgloss.NewStyle().Background(cBarBg).Foreground(cAccent)

	sTabActive = lipgloss.NewStyle().Bold(true).Foreground(cOnColor).Background(cAccent).Padding(0, 2)
	sTabInactive = lipgloss.NewStyle().Foreground(cMuted).Padding(0, 2)
	sTabBadge = lipgloss.NewStyle().Bold(true).Foreground(cNew)
	sRule = lipgloss.NewStyle().Foreground(cFaint)

	sOwner = lipgloss.NewStyle().Foreground(cMuted)
	sRepo = lipgloss.NewStyle().Bold(true).Foreground(cText)
	sRepoSel = lipgloss.NewStyle().Bold(true).Foreground(cAccent)
	sCursor = lipgloss.NewStyle().Foreground(cAccent).Bold(true)
	sNewBadge = lipgloss.NewStyle().Bold(true).Foreground(cOnColor).Background(cNew).Padding(0, 1)
	sStars = lipgloss.NewStyle().Foreground(cStar).Bold(true)
	sMuted = lipgloss.NewStyle().Foreground(cMuted)
	sDesc = lipgloss.NewStyle().Foreground(cText)
	sErr = lipgloss.NewStyle().Foreground(cErr).Bold(true)
	sKey = lipgloss.NewStyle().Foreground(cAccent2).Bold(true)
	sTopic = lipgloss.NewStyle().Foreground(cAccent).Background(cSelBg).Padding(0, 1)
	sMark = lipgloss.NewStyle().Foreground(cMark).Bold(true)
	sSection = lipgloss.NewStyle().Foreground(cAccent).Bold(true)
	sUp = lipgloss.NewStyle().Foreground(cUp).Bold(true)
	sSpk = lipgloss.NewStyle().Foreground(cUp)
}

func init() { buildStyles() }

// Couleurs officielles GitHub des langages les plus courants.
var langColors = map[string]string{
	"Go": "#00ADD8", "Rust": "#DEA584", "Python": "#3572A5", "TypeScript": "#3178C6",
	"JavaScript": "#F1E05A", "C++": "#F34B7D", "C": "#A8B9CC", "Java": "#B07219",
	"Zig": "#EC915C", "Swift": "#F05138", "Kotlin": "#A97BFF", "Ruby": "#CC342D",
	"Shell": "#89E051", "HTML": "#E34C26", "CSS": "#663399", "Jupyter Notebook": "#DA5B0B",
	"Lua": "#6C7EE1", "Elixir": "#6E4A7E", "PHP": "#4F5D95", "C#": "#178600",
	"Dart": "#00B4AB", "Scala": "#C22D40", "Haskell": "#5E5086", "Nix": "#7E7EFF",
	"Vue": "#41B883", "Svelte": "#FF3E00", "OCaml": "#EF7A08", "Julia": "#A270BA",
}

func langBadge(lang string) string { return langBadgeOn(lang, lipgloss.NoColor{}) }

// langBadgeOn : pastille du langage sur le fond bg (ligne sélectionnée).
func langBadgeOn(lang string, bg lipgloss.TerminalColor) string {
	muted := sMuted.Background(bg)
	if lang == "" {
		return muted.Render("○ —")
	}
	col, ok := langColors[lang]
	if !ok {
		col = "#8A86A3"
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color(col)).Background(bg).Render("●") + muted.Render(" "+lang)
}

// humanStars : 1234 → 1.2k
func humanStars(n int) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1e6)
	case n >= 10_000:
		return fmt.Sprintf("%dk", n/1000)
	case n >= 1000:
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	}
	return fmt.Sprint(n)
}

// humanGain : 1240 → « +1.2k », -3 → « -3 ».
func humanGain(n int) string {
	if n < 0 {
		return "-" + humanStars(-n)
	}
	return "+" + humanStars(n)
}

// spanLabel : durée de mesure arrondie, « 7 j » ou « 5 h ».
func spanLabel(d time.Duration) string {
	if d >= 24*time.Hour {
		return fmt.Sprintf("%d j", int((d+12*time.Hour)/(24*time.Hour)))
	}
	return fmt.Sprintf("%d h", max(int(d.Hours()), 1))
}

// sparkline dessine l'évolution des étoiles avec des blocs ▁▂▃▄▅▆▇█
// (les « width » derniers relevés).
func sparkline(pts []StarPoint, width int) string {
	if len(pts) < 2 || width < 2 {
		return ""
	}
	if len(pts) > width {
		pts = pts[len(pts)-width:]
	}
	lo, hi := pts[0].N, pts[0].N
	for _, p := range pts {
		lo, hi = min(lo, p.N), max(hi, p.N)
	}
	bars := []rune("▁▂▃▄▅▆▇█")
	out := make([]rune, len(pts))
	for i, p := range pts {
		lvl := 0
		if hi > lo {
			lvl = (p.N - lo) * (len(bars) - 1) / (hi - lo)
		}
		out[i] = bars[lvl]
	}
	return string(out)
}
