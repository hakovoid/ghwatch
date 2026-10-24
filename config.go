package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

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
