package main

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
)

// Palette : s'adapte automatiquement aux terminaux clairs et sombres.
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
	cOnColor = lipgloss.Color("#FFFFFF")
)

var (
	sLogo = lipgloss.NewStyle().Bold(true).Foreground(cOnColor).Background(cAccent).Padding(0, 1)
	sBar  = lipgloss.NewStyle().Background(cBarBg).Foreground(cMuted)

	sTabActive   = lipgloss.NewStyle().Bold(true).Foreground(cOnColor).Background(cAccent).Padding(0, 2)
	sTabInactive = lipgloss.NewStyle().Foreground(cMuted).Padding(0, 2)
	sTabBadge    = lipgloss.NewStyle().Bold(true).Foreground(cNew)
	sRule        = lipgloss.NewStyle().Foreground(cFaint)

	sOwner    = lipgloss.NewStyle().Foreground(cMuted)
	sRepo     = lipgloss.NewStyle().Bold(true).Foreground(cText)
	sRepoSel  = lipgloss.NewStyle().Bold(true).Foreground(cAccent)
	sCursor   = lipgloss.NewStyle().Foreground(cAccent).Bold(true)
	sNewBadge = lipgloss.NewStyle().Bold(true).Foreground(cOnColor).Background(cNew).Padding(0, 1)
	sStars    = lipgloss.NewStyle().Foreground(cStar).Bold(true)
	sMuted    = lipgloss.NewStyle().Foreground(cMuted)
	sDesc     = lipgloss.NewStyle().Foreground(cText)
	sErr      = lipgloss.NewStyle().Foreground(cErr).Bold(true)
	sKey      = lipgloss.NewStyle().Foreground(cAccent2).Bold(true)
	sTopic    = lipgloss.NewStyle().Foreground(cAccent).Background(cSelBg).Padding(0, 1)
)

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

func langBadge(lang string) string {
	if lang == "" {
		return sMuted.Render("○ —")
	}
	col, ok := langColors[lang]
	if !ok {
		col = "#8A86A3"
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color(col)).Render("●") + " " + sMuted.Render(lang)
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
