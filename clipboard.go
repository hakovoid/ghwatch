package main

import (
	"github.com/atotto/clipboard"
	"github.com/muesli/termenv"
)

// copyToClipboard copie le texte dans le presse-papiers du système
// (pbcopy, xclip/xsel/wl-copy, Windows). À défaut, il passe par le terminal
// (séquence OSC 52), ce qui marche aussi à travers SSH dans la plupart des
// terminaux récents.
var copyToClipboard = func(s string) (via string) {
	if err := clipboard.WriteAll(s); err == nil {
		return "presse-papiers"
	}
	termenv.Copy(s)
	return "terminal"
}
