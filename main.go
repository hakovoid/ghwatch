// ghwatch : veille des dépôts GitHub populaires / récents par thème.
//
// Usage :
//
//	ghwatch                      # tourne en continu, vérifie tous les `interval`
//	ghwatch -once                # une seule vérification puis quitte (pour cron)
//	ghwatch -config autre.json   # autre fichier de config
//	ghwatch -interval 6h         # remplace l'interval de la config
//	ghwatch -all                 # affiche aussi les dépôts déjà vus
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// ---------- Configuration ----------

type Config struct {
	Interval   string  `json:"interval"`    // ex: "6h", "1d", "1w", "30m"
	StateFile  string  `json:"state_file"`  // dépôts déjà vus
	ReportsDir string  `json:"reports_dir"` // rapports Markdown ("" = désactivé)
	Watches    []Watch `json:"watches"`
}

type Watch struct {
	Name          string   `json:"name"`
	Query         string   `json:"query"`          // requête GitHub brute (optionnelle)
	Topics        []string `json:"topics"`         // ex: [llm, agents]
	Language      string   `json:"language"`       // ex: go
	MinStars      int      `json:"min_stars"`      // ex: 50
	CreatedWithin string   `json:"created_within"` // ex: "7d" → créés ces 7 derniers jours
	PushedWithin  string   `json:"pushed_within"`  // ex: "2d" → actifs récemment
	Sort          string   `json:"sort"`           // stars | forks | updated (défaut: stars)
	Limit         int      `json:"limit"`          // défaut: 10, max: 100
}

func loadConfig(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c Config
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("config invalide: %w", err)
	}
	if c.Interval == "" {
		c.Interval = "24h"
	}
	if c.StateFile == "" {
		c.StateFile = "ghwatch-state.json"
	}
	if len(c.Watches) == 0 {
		return nil, fmt.Errorf("aucune veille définie dans 'watches'")
	}
	for i := range c.Watches {
		w := &c.Watches[i]
		if w.Limit <= 0 {
			w.Limit = 10
		}
		if w.Limit > 100 {
			w.Limit = 100
		}
		if w.Sort == "" {
			w.Sort = "stars"
		}
		if w.Name == "" {
			w.Name = fmt.Sprintf("veille-%d", i+1)
		}
	}
	return &c, nil
}

// parseDuration accepte les durées Go (30m, 6h) plus les jours (1d) et semaines (2w).
func parseDuration(s string) (time.Duration, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	if n := len(s); n > 1 {
		unit := s[n-1]
		if unit == 'd' || unit == 'w' {
			v, err := strconv.ParseFloat(s[:n-1], 64)
			if err != nil {
				return 0, fmt.Errorf("durée invalide %q", s)
			}
			d := time.Duration(v * float64(24*time.Hour))
			if unit == 'w' {
				d *= 7
			}
			return d, nil
		}
	}
	return time.ParseDuration(s)
}

// buildQuery transforme une veille en requête de recherche GitHub.
func buildQuery(w Watch, now time.Time) (string, error) {
	var parts []string
	if w.Query != "" {
		parts = append(parts, w.Query)
	}
	for _, t := range w.Topics {
		parts = append(parts, "topic:"+t)
	}
	if w.Language != "" {
		parts = append(parts, "language:"+w.Language)
	}
	if w.MinStars > 0 {
		parts = append(parts, fmt.Sprintf("stars:>=%d", w.MinStars))
	}
	if w.CreatedWithin != "" {
		d, err := parseDuration(w.CreatedWithin)
		if err != nil {
			return "", err
		}
		parts = append(parts, "created:>="+now.Add(-d).Format("2006-01-02"))
	}
	if w.PushedWithin != "" {
		d, err := parseDuration(w.PushedWithin)
		if err != nil {
			return "", err
		}
		parts = append(parts, "pushed:>="+now.Add(-d).Format("2006-01-02"))
	}
	if len(parts) == 0 {
		return "", fmt.Errorf("veille %q : aucun critère", w.Name)
	}
	return strings.Join(parts, " "), nil
}

// ---------- API GitHub ----------

type Repo struct {
	FullName    string    `json:"full_name"`
	HTMLURL     string    `json:"html_url"`
	Description string    `json:"description"`
	Stars       int       `json:"stargazers_count"`
	Forks       int       `json:"forks_count"`
	Language    string    `json:"language"`
	Topics      []string  `json:"topics"`
	CreatedAt   time.Time `json:"created_at"`
	PushedAt    time.Time `json:"pushed_at"`
}

type searchResp struct {
	TotalCount int    `json:"total_count"`
	Items      []Repo `json:"items"`
}

var httpClient = &http.Client{Timeout: 30 * time.Second}

// apiBase peut être changé via GITHUB_API_URL (GitHub Enterprise, tests).
func apiBase() string {
	if v := os.Getenv("GITHUB_API_URL"); v != "" {
		return strings.TrimRight(v, "/")
	}
	return "https://api.github.com"
}

func searchRepos(q, sortBy string, limit int, token string) ([]Repo, error) {
	u := apiBase() + "/search/repositories?" + url.Values{
		"q":        {q},
		"sort":     {sortBy},
		"order":    {"desc"},
		"per_page": {strconv.Itoa(limit)},
	}.Encode()

	req, _ := http.NewRequest("GET", u, nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "ghwatch")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var r searchResp
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, err
	}
	return r.Items, nil
}

// ---------- État (dépôts déjà vus) ----------

type State struct {
	Seen map[string]time.Time `json:"seen"` // full_name → première fois vu
}

func loadState(path string) *State {
	s := &State{Seen: map[string]time.Time{}}
	if b, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(b, s)
		if s.Seen == nil {
			s.Seen = map[string]time.Time{}
		}
	}
	return s
}

func (s *State) save(path string) error {
	b, _ := json.MarshalIndent(s, "", "  ")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// ---------- Vérification ----------

type result struct {
	Watch Watch
	Query string
	Repos []Repo
	Err   error
}

func check(cfg *Config, token string, showAll bool) {
	now := time.Now()
	state := loadState(cfg.StateFile)
	var results []result
	newCount := 0

	for i, w := range cfg.Watches {
		if i > 0 {
			time.Sleep(2 * time.Second) // reste sous la limite de l'API Search
		}
		q, err := buildQuery(w, now)
		if err != nil {
			results = append(results, result{Watch: w, Err: err})
			continue
		}
		repos, err := searchRepos(q, w.Sort, w.Limit, token)
		r := result{Watch: w, Query: q, Err: err}
		for _, repo := range repos {
			_, seen := state.Seen[repo.FullName]
			if !seen {
				state.Seen[repo.FullName] = now
				newCount++
			}
			if !seen || showAll {
				r.Repos = append(r.Repos, repo)
			}
		}
		results = append(results, r)
	}

	if err := state.save(cfg.StateFile); err != nil {
		log.Printf("impossible d'enregistrer l'état: %v", err)
	}

	report := renderMarkdown(results, now, newCount)
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

func renderMarkdown(results []result, now time.Time, newCount int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Veille GitHub — %s\n\n", now.Format("02/01/2006 15:04"))
	fmt.Fprintf(&b, "%d nouveau(x) dépôt(s).\n", newCount)

	for _, r := range results {
		fmt.Fprintf(&b, "\n## %s\n", r.Watch.Name)
		if r.Query != "" {
			fmt.Fprintf(&b, "_Requête : `%s`_\n\n", r.Query)
		}
		if r.Err != nil {
			fmt.Fprintf(&b, "⚠️ Erreur : %v\n", r.Err)
			continue
		}
		if len(r.Repos) == 0 {
			b.WriteString("Rien de nouveau.\n")
			continue
		}
		sort.SliceStable(r.Repos, func(i, j int) bool { return r.Repos[i].Stars > r.Repos[j].Stars })
		for _, repo := range r.Repos {
			desc := strings.TrimSpace(repo.Description)
			if desc == "" {
				desc = "(pas de description)"
			}
			lang := repo.Language
			if lang == "" {
				lang = "?"
			}
			fmt.Fprintf(&b, "- **[%s](%s)** ★ %d · %s · créé le %s\n  %s\n",
				repo.FullName, repo.HTMLURL, repo.Stars, lang,
				repo.CreatedAt.Format("02/01/2006"), desc)
		}
	}
	return b.String()
}

// ---------- main ----------

func main() {
	cfgPath := flag.String("config", "config.json", "fichier de configuration")
	intervalFlag := flag.String("interval", "", "fréquence (ex: 30m, 6h, 1d, 1w) — remplace celle de la config")
	once := flag.Bool("once", false, "une seule vérification puis quitter")
	showAll := flag.Bool("all", false, "afficher aussi les dépôts déjà vus")
	flag.Parse()

	// Charge la config et calcule l'intervalle (le flag -interval a priorité).
	load := func() (*Config, time.Duration, error) {
		c, err := loadConfig(*cfgPath)
		if err != nil {
			return nil, 0, err
		}
		if *intervalFlag != "" {
			c.Interval = *intervalFlag
		}
		d, err := parseDuration(c.Interval)
		if err != nil || d < time.Minute {
			return nil, 0, fmt.Errorf("interval invalide %q (minimum 1m)", c.Interval)
		}
		return c, d, nil
	}

	cfg, interval, err := load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		log.Println("GITHUB_TOKEN absent : limite de l'API réduite (10 recherches/min)")
	}

	check(cfg, token, *showAll)
	if *once {
		return
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	for {
		next := time.Now().Add(interval)
		log.Printf("prochaine vérification : %s (dans %s) — Ctrl+C pour arrêter",
			next.Format("02/01 15:04"), interval)
		select {
		case <-time.After(interval):
			// Recharge la config à chaque passage : thèmes ET fréquence
			// modifiés sans redémarrer.
			if c, d, err := load(); err == nil {
				cfg, interval = c, d
			} else {
				log.Printf("config non rechargée (ancienne conservée): %v", err)
			}
			check(cfg, token, *showAll)
		case <-stop:
			log.Println("arrêt.")
			return
		}
	}
}
