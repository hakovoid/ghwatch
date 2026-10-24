package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

const historyMax = 30

// historyPath : l'historique est rangé à côté du fichier d'état.
func historyPath(cfg *Config) string {
	return filepath.Join(filepath.Dir(cfg.StateFile), "ghwatch-history.json")
}

// loadHistory renvoie les recherches passées, la plus récente en premier.
func loadHistory(path string) []string {
	var h []string
	if b, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(b, &h)
	}
	return h
}

func saveHistory(path string, h []string) error {
	b, _ := json.MarshalIndent(h, "", "  ")
	return os.WriteFile(path, b, 0o644)
}

// pushHistory place q en tête, sans doublon, en gardant les historyMax dernières.
func pushHistory(h []string, q string) []string {
	q = strings.TrimSpace(q)
	out := []string{q}
	for _, old := range h {
		if !strings.EqualFold(old, q) {
			out = append(out, old)
		}
	}
	return out[:min(len(out), historyMax)]
}
