package main

import "strings"

// Les descriptions et README GitHub contiennent souvent des emoji (🚀, ❤️,
// 👨‍💻…). Selon le terminal et la police, ils occupent 1 ou 2 colonnes, alors
// que l'interface compte la largeur à l'avance : une ligne qui déborde est
// repliée par le terminal et décale tout l'écran (lignes en double). On les
// retire donc du texte affiché ; les accents, le CJK et la ponctuation restent.

// isEmoji : pictogrammes, symboles « emoji », sélecteurs de variante et
// caractères de liaison qui n'ont pas de largeur fiable dans un terminal.
func isEmoji(r rune) bool {
	switch {
	case r >= 0x1F000 && r <= 0x1FAFF: // emoji, drapeaux, cartes, mahjong…
	case r >= 0x2600 && r <= 0x27BF: // symboles divers et dingbats (⚡ ✨ ✅ ❤)
	case r >= 0x2B00 && r <= 0x2BFF: // flèches et étoiles (⭐ ⬆)
	case r >= 0x2300 && r <= 0x23FF: // symboles techniques (⌚ ⏩ ⏰)
	case r >= 0xFE00 && r <= 0xFE0F: // sélecteurs de variante (texte / emoji)
	case r >= 0xE0000 && r <= 0xE007F: // étiquettes (drapeaux régionaux)
	case r == 0x200D, r == 0x20E3: // liaison (ZWJ), touche ⃣
	default:
		return false
	}
	return true
}

// displayText retire les emoji d'un texte et nettoie les espaces laissés
// (sans toucher aux retours à la ligne).
func displayText(s string) string {
	if !strings.ContainsFunc(s, isEmoji) {
		return s
	}
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		rest := strings.TrimLeft(l, " \t")
		lead := l[:len(l)-len(rest)] // l'indentation d'origine est conservée
		lines[i] = lead + strings.Join(strings.Fields(strings.Map(func(r rune) rune {
			if isEmoji(r) {
				return -1
			}
			return r
		}, rest)), " ")
	}
	return strings.Join(lines, "\n")
}
