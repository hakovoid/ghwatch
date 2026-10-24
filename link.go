package main

import "github.com/charmbracelet/x/ansi"

// linksEnabled active les liens cliquables (séquence OSC 8). Les terminaux
// qui ne la gèrent pas l'ignorent ; -links=false la désactive si besoin.
var linksEnabled = true

// hyperlink rend « text » (déjà stylé) cliquable vers url.
func hyperlink(url, text string) string {
	if !linksEnabled || url == "" {
		return text
	}
	return ansi.SetHyperlink(url) + text + ansi.ResetHyperlink()
}
