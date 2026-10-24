package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Item est un dépôt trouvé, avec son statut de nouveauté.
type Item struct {
	Repo
	New bool // jamais vu avant ce passage
}

// Result regroupe les dépôts trouvés pour une veille.
type Result struct {
	Watch Watch
	Query string
	Items []Item
	Err   error
}

// NewCount renvoie le nombre de dépôts nouveaux dans ce résultat.
func (r Result) NewCount() int {
	n := 0
	for _, it := range r.Items {
		if it.New {
			n++
		}
	}
	return n
}

// searchPause espace les appels à l'API Search pour rester sous sa limite.
var searchPause = 2 * time.Second

// Fetch interroge GitHub pour chaque veille, marque les nouveautés et met à
// jour le fichier d'état. C'est le cœur partagé par la CLI et la TUI.
func Fetch(cfg *Config, token string, now time.Time) []Result {
	state := loadState(cfg.StateFile)
	results := make([]Result, 0, len(cfg.Watches))

	for i, w := range cfg.Watches {
		if i > 0 {
			time.Sleep(searchPause)
		}
		q, err := buildQuery(w, now)
		if err != nil {
			results = append(results, Result{Watch: w, Err: err})
			continue
		}
		repos, err := searchRepos(q, w.Sort, w.Limit, token)
		r := Result{Watch: w, Query: q, Err: err}
		for _, repo := range repos {
			_, seen := state.Seen[repo.FullName]
			if !seen {
				state.Seen[repo.FullName] = now
			}
			r.Items = append(r.Items, Item{Repo: repo, New: !seen})
		}
		results = append(results, r)
	}

	if err := state.save(cfg.StateFile); err != nil {
		log.Printf("impossible d'enregistrer l'état: %v", err)
	}
	return results
}

// check est le mode texte : affiche le rapport et l'enregistre en Markdown.
func check(cfg *Config, token string, showAll bool) {
	now := time.Now()
	results := Fetch(cfg, token, now)
	report := renderMarkdown(results, now, showAll)
	fmt.Println(report)

	if cfg.ReportsDir != "" {
		if err := os.MkdirAll(cfg.ReportsDir, 0o755); err == nil {
			name := filepath.Join(cfg.ReportsDir, now.Format("2006-01-02_15h04")+".md")
			if err := os.WriteFile(name, []byte(report), 0o644); err != nil {
				log.Printf("écriture du rapport: %v", err)
			} else {
				log.Printf("rapport enregistré : %s", name)
			}
		}
	}
}

func renderMarkdown(results []Result, now time.Time, showAll bool) string {
	var b strings.Builder
	total := 0
	for _, r := range results {
		total += r.NewCount()
	}
	fmt.Fprintf(&b, "# Veille GitHub — %s\n\n", now.Format("02/01/2006 15:04"))
	fmt.Fprintf(&b, "%d nouveau(x) dépôt(s).\n", total)

	for _, r := range results {
		fmt.Fprintf(&b, "\n## %s\n", r.Watch.Name)
		if r.Query != "" {
			fmt.Fprintf(&b, "_Requête : `%s`_\n\n", r.Query)
		}
		if r.Err != nil {
			fmt.Fprintf(&b, "⚠️ Erreur : %v\n", r.Err)
			continue
		}
		var items []Item
		for _, it := range r.Items {
			if it.New || showAll {
				items = append(items, it)
			}
		}
		if len(items) == 0 {
			b.WriteString("Rien de nouveau.\n")
			continue
		}
		sort.SliceStable(items, func(i, j int) bool { return items[i].Stars > items[j].Stars })
		for _, it := range items {
			desc := strings.TrimSpace(it.Description)
			if desc == "" {
				desc = "(pas de description)"
			}
			lang := it.Language
			if lang == "" {
				lang = "?"
			}
			fmt.Fprintf(&b, "- **[%s](%s)** ★ %d · %s · créé le %s\n  %s\n",
				it.FullName, it.HTMLURL, it.Stars, lang,
				it.CreatedAt.Format("02/01/2006"), desc)
		}
	}
	return b.String()
}
