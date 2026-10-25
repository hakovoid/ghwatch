package main

import (
	"regexp"
	"strings"
)

// Extrait du README affiché dans le panneau de détail : le début du texte,
// débarrassé du Markdown et du HTML (badges, images, blocs de code, tableaux).

const readmeMaxChars = 2500

var (
	reComment   = regexp.MustCompile(`(?s)<!--.*?-->`)
	reFence     = regexp.MustCompile("(?ms)^[ \t]*(```|~~~).*?^[ \t]*(```|~~~)[^\n]*$")
	reHTMLBlock = regexp.MustCompile(`(?is)<(picture|svg|table|details|style|script)\b.*?</(picture|svg|table|details|style|script)>`)
	reTag       = regexp.MustCompile(`<[^>]+>`)
	reImage     = regexp.MustCompile(`!\[[^\]]*\]\([^)]*\)|!\[[^\]]*\]\[[^\]]*\]`)
	reEmptyLink = regexp.MustCompile(`\[\s*\]\([^)]*\)|\[\s*\]\[[^\]]*\]`)
	reLink      = regexp.MustCompile(`\[([^\]]+)\]\([^)]*\)|\[([^\]]+)\]\[[^\]]*\]`)
	reRefDef    = regexp.MustCompile(`(?m)^[ \t]*\[[^\]]+\]:[ \t]+\S+.*$`)
	reHeading   = regexp.MustCompile(`(?m)^[ \t]{0,3}#{1,6}[ \t]*(.*?)[ \t]*#*[ \t]*$`)
	reSetext    = regexp.MustCompile(`(?m)^[ \t]*(=+|-+)[ \t]*$`)
	reEmphasis  = regexp.MustCompile("\\*\\*|__|`")
	reBullet    = regexp.MustCompile(`(?m)^([ \t]*)[*+-][ \t]+`)
	reEntity    = strings.NewReplacer("&nbsp;", " ", "&amp;", "&", "&lt;", "<", "&gt;", ">", "&quot;", `"`, "&#39;", "'")
)

// readmeText convertit un README Markdown en texte brut d'au plus max caractères.
func readmeText(md string, max int) string {
	s := strings.ReplaceAll(md, "\r\n", "\n")
	s = reComment.ReplaceAllString(s, "")
	s = reFence.ReplaceAllString(s, "")
	s = reHTMLBlock.ReplaceAllString(s, "")
	s = reImage.ReplaceAllString(s, "")
	s = reEmptyLink.ReplaceAllString(s, "")
	s = reLink.ReplaceAllString(s, "$1$2")
	s = reRefDef.ReplaceAllString(s, "")
	s = reTag.ReplaceAllString(s, "")
	s = reHeading.ReplaceAllString(s, "$1")
	s = reSetext.ReplaceAllString(s, "")
	s = reEmphasis.ReplaceAllString(s, "")
	s = reBullet.ReplaceAllString(s, "$1• ")
	s = reEntity.Replace(s)

	// lignes utiles seulement : pas de tableaux, de séparateurs ni de lignes vides en série
	var out []string
	blank := true
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimRight(line, " \t")
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "|") || strings.Trim(t, "-*_=| ") == "" && t != "" {
			continue
		}
		if t == "" {
			if !blank {
				out = append(out, "")
			}
			blank = true
			continue
		}
		out = append(out, line)
		blank = false
	}
	s = strings.TrimSpace(displayText(strings.Join(out, "\n")))

	if r := []rune(s); len(r) > max {
		s = string(r[:max])
		if i := strings.LastIndexAny(s, " \n"); i > max*3/4 {
			s = s[:i]
		}
		s = strings.TrimSpace(s) + " …"
	}
	return s
}
