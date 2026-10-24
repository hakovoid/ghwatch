package main

import (
	"encoding/json"
	"os"
	"time"
)

// State mémorise les dépôts déjà vus pour ne signaler que les nouveautés.
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
