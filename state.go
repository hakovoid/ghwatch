package main

import (
	"encoding/json"
	"os"
	"time"
)

// State mémorise les dépôts déjà vus pour ne signaler que les nouveautés,
// et l'historique de leurs étoiles pour mesurer leur progression.
type State struct {
	Seen  map[string]time.Time   `json:"seen"`            // full_name → première fois vu
	Stars map[string][]StarPoint `json:"stars,omitempty"` // full_name → relevés d'étoiles
}

// StarPoint est un relevé du nombre d'étoiles à un instant donné.
type StarPoint struct {
	T time.Time `json:"t"`
	N int       `json:"n"`
}

const (
	starMergeWindow = time.Hour           // relevés plus rapprochés : on remplace le dernier
	starKeep        = 90 * 24 * time.Hour // historique conservé
	minTrendSpan    = time.Hour           // en dessous, la progression n'a pas de sens
)

func loadState(path string) *State {
	s := &State{}
	if b, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(b, s)
	}
	if s.Seen == nil {
		s.Seen = map[string]time.Time{}
	}
	if s.Stars == nil {
		s.Stars = map[string][]StarPoint{}
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

// recordStars ajoute un relevé. Deux relevés à moins d'une heure d'écart
// (actualisations manuelles, dépôt présent dans deux veilles) n'en font
// qu'un ; les relevés de plus de 90 jours sont oubliés.
func (s *State) recordStars(name string, stars int, now time.Time) {
	pts := s.Stars[name]
	if n := len(pts); n > 0 && now.Sub(pts[n-1].T) < starMergeWindow {
		pts[n-1] = StarPoint{T: now, N: stars}
	} else {
		pts = append(pts, StarPoint{T: now, N: stars})
	}
	cut := 0
	for cut < len(pts)-1 && now.Sub(pts[cut].T) > starKeep {
		cut++
	}
	s.Stars[name] = pts[cut:]
}

// Trend décrit la progression d'un dépôt sur la fenêtre demandée.
type Trend struct {
	Gain   int           // étoiles gagnées
	Span   time.Duration // durée réellement mesurée (≤ fenêtre au début)
	OK     bool          // assez d'historique pour mesurer
	Points []StarPoint   // historique, relevé courant compris (pour le graphique)
}

// trend compare le nombre d'étoiles actuel à l'historique, sans le modifier.
// La référence est le dernier relevé datant d'au moins « window » ; à défaut
// (historique plus court), le plus ancien relevé disponible.
func (s *State) trend(name string, stars int, now time.Time, window time.Duration) Trend {
	var past []StarPoint
	for _, p := range s.Stars[name] {
		if now.Sub(p.T) >= minTrendSpan { // ignore un relevé quasi simultané
			past = append(past, p)
		}
	}
	t := Trend{Points: append(append([]StarPoint{}, past...), StarPoint{T: now, N: stars})}
	if len(past) == 0 {
		return t
	}
	base := past[0]
	for _, p := range past {
		if now.Sub(p.T) >= window {
			base = p
		}
	}
	t.Gain, t.Span, t.OK = stars-base.N, now.Sub(base.T), true
	return t
}
