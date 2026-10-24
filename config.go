package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Interval    string  `json:"interval"`               // ex: "6h", "1d", "1w", "30m"
	StateFile   string  `json:"state_file"`             // dépôts déjà vus
	ReportsDir  string  `json:"reports_dir,omitempty"`  // rapports Markdown ("" = désactivé)
	TrendWindow string  `json:"trend_window,omitempty"` // période de progression des étoiles (défaut 7d)
	Watches     []Watch `json:"watches"`
}

type Watch struct {
	Name          string   `json:"name"`
	Query         string   `json:"query,omitempty"`          // requête GitHub brute (optionnelle)
	Any           []string `json:"any,omitempty"`            // termes combinés en OU : une recherche par terme, résultats fusionnés
	Topics        []string `json:"topics,omitempty"`         // ex: [llm, agents]
	Language      string   `json:"language,omitempty"`       // ex: go
	MinStars      int      `json:"min_stars,omitempty"`      // ex: 50
	CreatedWithin string   `json:"created_within,omitempty"` // ex: "7d" → créés ces 7 derniers jours
	PushedWithin  string   `json:"pushed_within,omitempty"`  // ex: "2d" → actifs récemment
	Sort          string   `json:"sort,omitempty"`           // stars | forks | updated (défaut: stars)
	Limit         int      `json:"limit,omitempty"`          // défaut: 10, max: 100
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
	if c.TrendWindow != "" {
		if d, err := parseDuration(c.TrendWindow); err != nil || d < time.Hour {
			return nil, fmt.Errorf("trend_window invalide %q (minimum 1h)", c.TrendWindow)
		}
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

// trendWindow renvoie la période de mesure de la progression (7 jours par défaut).
func (c *Config) trendWindow() time.Duration {
	if d, err := parseDuration(c.TrendWindow); err == nil && d >= time.Hour {
		return d
	}
	return 7 * 24 * time.Hour
}

// loadConfigWithInterval charge la config, applique un éventuel intervalle
// imposé en ligne de commande et renvoie l'intervalle calculé.
func loadConfigWithInterval(path, override string) (*Config, time.Duration, error) {
	c, err := loadConfig(path)
	if err != nil {
		return nil, 0, err
	}
	if override != "" {
		c.Interval = override
	}
	d, err := parseDuration(c.Interval)
	if err != nil || d < time.Minute {
		return nil, 0, fmt.Errorf("interval invalide %q (minimum 1m)", c.Interval)
	}
	return c, d, nil
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

// buildQueries renvoie les requêtes GitHub d'une veille : une seule en
// général, ou une par terme de « any » (chacune avec les critères communs).
func buildQueries(w Watch, now time.Time) ([]string, error) {
	var terms []string
	for _, t := range w.Any {
		if t = strings.TrimSpace(t); t != "" {
			terms = append(terms, t)
		}
	}
	if len(terms) == 0 {
		q, err := buildQuery(w, now)
		if err != nil {
			return nil, err
		}
		return []string{q}, nil
	}
	base, err := buildQuery(w, now)
	if err != nil && !errors.Is(err, errNoCriteria) {
		return nil, err
	}
	qs := make([]string, 0, len(terms))
	for _, t := range terms {
		qs = append(qs, strings.TrimSpace(t+" "+base))
	}
	return qs, nil
}

var errNoCriteria = errors.New("aucun critère")

// buildQuery transforme les critères communs d'une veille en requête GitHub.
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
		return "", fmt.Errorf("veille %q : %w", w.Name, errNoCriteria)
	}
	return strings.Join(parts, " "), nil
}

// errWatchExists : une veille avec la même requête existe déjà.
var errWatchExists = fmt.Errorf("cette recherche est déjà une veille")

// appendWatch ajoute une veille au fichier de config, sans y écrire les
// valeurs par défaut des autres veilles.
func appendWatch(path string, w Watch) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var raw Config
	if err := json.Unmarshal(b, &raw); err != nil {
		return fmt.Errorf("config invalide: %w", err)
	}
	for _, existing := range raw.Watches {
		if w.Query != "" && strings.EqualFold(strings.TrimSpace(existing.Query), strings.TrimSpace(w.Query)) {
			return errWatchExists
		}
	}
	raw.Watches = append(raw.Watches, w)
	// SetEscapeHTML(false) : garde « stars:>100 » lisible au lieu de « \u003e ».
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(raw); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, buf.Bytes(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
