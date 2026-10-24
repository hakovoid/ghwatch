package main

import (
	"encoding/json"
	"os"
	"sort"
	"sync"
	"time"
)

// State est la mémoire locale de ghwatch : dépôts déjà vus, historique de
// leurs étoiles, derniers détails connus, résultats de chaque veille et
// marque-pages (favoris, à voir). Elle permet de rouvrir la TUI sans
// interroger GitHub tant que l'intervalle n'est pas écoulé.
//
// La TUI la partage entre l'affichage et les tâches de fond : les méthodes
// exportées à la TUI prennent le verrou, les autres supposent qu'il est pris.
type State struct {
	mu sync.Mutex

	Seen    map[string]time.Time            `json:"seen"`              // full_name → première fois vu
	Stars   map[string][]StarPoint          `json:"stars,omitempty"`   // full_name → relevés d'étoiles
	Repos   map[string]*StoredRepo          `json:"repos,omitempty"`   // full_name → derniers détails connus
	Hits    map[string]map[string]time.Time `json:"hits,omitempty"`    // veille → full_name → dernière fois trouvé
	Checked map[string]time.Time            `json:"checked,omitempty"` // veille → dernière vérification
}

// StoredRepo : un dépôt tel que vu la dernière fois, avec ses marque-pages
// et l'extrait de son README.
type StoredRepo struct {
	Repo
	Fav      bool      `json:"fav,omitempty"`
	Later    bool      `json:"later,omitempty"`
	Readme   string    `json:"readme,omitempty"`   // extrait du README, en texte
	ReadmeAt time.Time `json:"readme_at,omitzero"` // zéro = jamais récupéré
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
	readmeMaxAge    = 30 * 24 * time.Hour // au-delà, le README est relu
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
	if s.Repos == nil {
		s.Repos = map[string]*StoredRepo{}
	}
	if s.Hits == nil {
		s.Hits = map[string]map[string]time.Time{}
	}
	if s.Checked == nil {
		s.Checked = map[string]time.Time{}
	}
	return s
}

func (s *State) save(path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
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

// ingest enregistre les dépôts trouvés par une veille lors d'une vérification.
func (s *State) ingest(w Watch, repos []Repo, now time.Time) {
	hits := s.Hits[w.Name]
	if hits == nil {
		hits = map[string]time.Time{}
		s.Hits[w.Name] = hits
	}
	for _, r := range repos {
		if _, ok := s.Seen[r.FullName]; !ok {
			s.Seen[r.FullName] = now
		}
		s.storeRepo(r)
		s.recordStars(r.FullName, r.Stars, now)
		hits[r.FullName] = now
	}
	s.Checked[w.Name] = now
}

// storeRepo met à jour les détails d'un dépôt sans perdre ses marque-pages.
func (s *State) storeRepo(r Repo) *StoredRepo {
	sr := s.Repos[r.FullName]
	if sr == nil {
		sr = &StoredRepo{}
		s.Repos[r.FullName] = sr
	}
	sr.Repo = r
	return sr
}

// prune oublie les dépôts qu'aucune veille n'a retrouvés depuis « keep »,
// sauf ceux mis en favori ou à voir.
func (s *State) prune(keep time.Duration, now time.Time) {
	used := map[string]bool{}
	for w, hits := range s.Hits {
		for name, at := range hits {
			if now.Sub(at) > keep {
				delete(hits, name)
			} else {
				used[name] = true
			}
		}
		if len(hits) == 0 {
			delete(s.Hits, w)
		}
	}
	for name, sr := range s.Repos {
		if !used[name] && !sr.Fav && !sr.Later {
			delete(s.Repos, name)
		}
	}
}

// due : la veille n'a jamais été vérifiée, ou l'intervalle est écoulé.
func (s *State) due(w Watch, interval time.Duration, now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	at, ok := s.Checked[w.Name]
	return !ok || now.Sub(at) >= interval
}

// schedule renvoie la dernière vérification des veilles de la config et la
// date de la prochaine (la plus proche échéance).
func (s *State) schedule(cfg *Config, interval time.Duration, now time.Time) (last, next time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, w := range cfg.Watches {
		at, ok := s.Checked[w.Name]
		if !ok {
			return last, now
		}
		if at.After(last) {
			last = at
		}
		if n := at.Add(interval); next.IsZero() || n.Before(next) {
			next = n
		}
	}
	return last, next
}

// results reconstruit les onglets à partir de la mémoire locale : tous les
// dépôts trouvés par chaque veille depuis « keep » (et pas seulement ceux
// du dernier passage), qui respectent encore ses critères de date.
// errs porte les erreurs du passage qui vient d'avoir lieu.
func (s *State) results(cfg *Config, errs map[string]Result, now time.Time) []Result {
	s.mu.Lock()
	defer s.mu.Unlock()
	window, keep := cfg.trendWindow(), cfg.keep()
	out := make([]Result, 0, len(cfg.Watches))
	for _, w := range cfg.Watches {
		r := errs[w.Name]
		r.Watch = w
		if qs, err := buildQueries(w, now); err == nil {
			r.Query = joinQueries(qs)
		} else if r.Err == nil {
			r.Err = err
		}
		r.CheckedAt = s.Checked[w.Name]
		created, pushed := sinceLimit(w.CreatedWithin, now), sinceLimit(w.PushedWithin, now)
		for name, at := range s.Hits[w.Name] {
			sr := s.Repos[name]
			if sr == nil || now.Sub(at) > keep ||
				sr.CreatedAt.Before(created) || sr.PushedAt.Before(pushed) {
				continue
			}
			r.Items = append(r.Items, s.item(sr, at, r.CheckedAt, window))
		}
		sortItems(r.Items, w.Sort)
		out = append(out, r)
	}
	return out
}

// sinceLimit : date limite d'un critère « created_within » / « pushed_within »
// (zéro si absent ou invalide).
func sinceLimit(within string, now time.Time) time.Time {
	if within == "" {
		return time.Time{}
	}
	d, err := parseDuration(within)
	if err != nil {
		return time.Time{}
	}
	return now.Add(-d).Truncate(24 * time.Hour)
}

// item : un dépôt mémorisé, vu pour la dernière fois à « at ». Il est
// nouveau s'il a été découvert lors de la dernière vérification de la veille.
func (s *State) item(sr *StoredRepo, at, checked time.Time, window time.Duration) Item {
	first := s.Seen[sr.FullName]
	return Item{
		Repo:  sr.Repo,
		New:   !checked.IsZero() && !first.Before(checked),
		Seen:  first,
		Trend: s.trend(sr.FullName, sr.Stars, at, window),
	}
}

// lastHit : dernière fois qu'une veille a trouvé ce dépôt.
func (s *State) lastHit(name string) time.Time {
	var last time.Time
	for _, hits := range s.Hits {
		if at := hits[name]; at.After(last) {
			last = at
		}
	}
	return last
}

// searchItems prépare les résultats d'une recherche ponctuelle : les dépôts
// jamais vus par les veilles sont marqués NEW, sans modifier l'état.
func (s *State) searchItems(repos []Repo, now time.Time, window time.Duration) []Item {
	s.mu.Lock()
	defer s.mu.Unlock()
	items := make([]Item, 0, len(repos))
	for _, r := range repos {
		first, seen := s.Seen[r.FullName]
		items = append(items, Item{Repo: r, New: !seen, Seen: first,
			Trend: s.trend(r.FullName, r.Stars, now, window)})
	}
	return items
}

// adopt enregistre les résultats d'une recherche comme ceux d'une nouvelle veille.
func (s *State) adopt(w Watch, repos []Repo, now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ingest(w, repos, now)
}

// Marque-pages.
type mark int

const (
	markFav mark = iota
	markLater
)

func (k mark) get(sr *StoredRepo) *bool {
	if k == markFav {
		return &sr.Fav
	}
	return &sr.Later
}

// toggle inverse un marque-page et renvoie sa nouvelle valeur. Le dépôt est
// mémorisé s'il ne l'était pas (résultat d'une recherche ponctuelle).
func (s *State) toggle(r Repo, k mark) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	sr := s.Repos[r.FullName]
	if sr == nil {
		sr = s.storeRepo(r)
	}
	p := k.get(sr)
	*p = !*p
	return *p
}

// marks renvoie les marque-pages d'un dépôt.
func (s *State) marks(name string) (fav, later bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sr := s.Repos[name]; sr != nil {
		return sr.Fav, sr.Later
	}
	return false, false
}

// marked renvoie les dépôts qui portent ce marque-page.
func (s *State) marked(k mark, window time.Duration) []Item {
	s.mu.Lock()
	defer s.mu.Unlock()
	var items []Item
	for _, sr := range s.Repos {
		if *k.get(sr) {
			at := s.lastHit(sr.FullName)
			if at.IsZero() {
				at = time.Now()
			}
			items = append(items, s.item(sr, at, time.Time{}, window))
		}
	}
	sortItems(items, "stars")
	return items
}

// readme renvoie l'extrait du README et s'il est à jour.
func (s *State) readme(name string, now time.Time) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sr := s.Repos[name]
	if sr == nil || sr.ReadmeAt.IsZero() {
		return "", false
	}
	return sr.Readme, now.Sub(sr.ReadmeAt) < readmeMaxAge
}

func (s *State) setReadme(r Repo, text string, now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sr := s.Repos[r.FullName]
	if sr == nil {
		sr = s.storeRepo(r)
	}
	sr.Readme, sr.ReadmeAt = text, now
}

// sortItems trie selon le critère d'une veille (stars par défaut).
func sortItems(items []Item, by string) {
	sort.SliceStable(items, func(i, j int) bool {
		switch by {
		case "forks":
			return items[i].Forks > items[j].Forks
		case "updated":
			return items[i].PushedAt.After(items[j].PushedAt)
		}
		return items[i].Stars > items[j].Stars
	})
}
