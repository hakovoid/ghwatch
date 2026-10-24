package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
)

// fakeSearch sert à chaque recherche les dépôts de *repos et compte les appels.
func fakeSearch(t *testing.T, repos *[]Repo) *atomic.Int32 {
	t.Helper()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/readme") {
			if strings.Contains(r.URL.Path, "/sans/") {
				http.NotFound(w, r)
				return
			}
			w.Write([]byte("# Titre\n\n![badge](https://x/b.svg)\n\nUn **outil** [génial](https://x).\n\n```sh\nmake\n```\n"))
			return
		}
		calls.Add(1)
		json.NewEncoder(w).Encode(searchResp{Items: *repos})
	}))
	t.Cleanup(srv.Close)
	t.Setenv("GITHUB_API_URL", srv.URL)
	searchPause, searchPauseAnon = 0, 0
	return &calls
}

func storeCfg(t *testing.T) *Config {
	return &Config{StateFile: t.TempDir() + "/s.json", Watches: []Watch{{Name: "w", Language: "go", Limit: 10}}}
}

func TestTUIUsesCacheUntilIntervalElapsed(t *testing.T) {
	repos := []Repo{{FullName: "a/one", Stars: 5, CreatedAt: t0}}
	calls := fakeSearch(t, &repos)
	cfg := storeCfg(t)
	Fetch(cfg, "", t0)
	if calls.Load() != 1 {
		t.Fatalf("appels = %d", calls.Load())
	}

	m := newModel(cfg, day)
	m.loadCached(t0.Add(3 * time.Hour))
	if m.loading {
		t.Fatal("intervalle non écoulé : la TUI ne doit pas interroger GitHub")
	}
	if got := names(m.results[0].Items); len(got) != 1 || got[0] != "a/one" {
		t.Fatalf("résultats mémorisés : %v", got)
	}
	if !m.lastRun.Equal(t0) || !m.nextRun.Equal(t0.Add(day)) {
		t.Errorf("planning : dernière %v, prochaine %v", m.lastRun, m.nextRun)
	}

	m = newModel(cfg, day)
	m.loadCached(t0.Add(25 * time.Hour))
	if !m.loading {
		t.Fatal("intervalle écoulé : une vérification est attendue")
	}
}

func TestResultsAccumulateAndExpire(t *testing.T) {
	repos := []Repo{{FullName: "a/old"}, {FullName: "a/both"}}
	fakeSearch(t, &repos)
	cfg := storeCfg(t)
	Fetch(cfg, "", t0)

	repos = []Repo{{FullName: "a/both"}, {FullName: "a/new"}}
	res := Fetch(cfg, "", t0.Add(day))[0]
	if got := strings.Join(names(res.Items), ","); len(res.Items) != 3 {
		t.Fatalf("les dépôts des passages précédents doivent rester : %s", got)
	}
	for _, it := range res.Items {
		if it.New != (it.FullName == "a/new") {
			t.Errorf("%s : New = %v", it.FullName, it.New)
		}
	}

	// 40 jours plus tard (keep = 30 j par défaut), seuls ceux retrouvés restent.
	repos = []Repo{{FullName: "a/new"}}
	res = Fetch(cfg, "", t0.Add(41*day))[0]
	if got := names(res.Items); len(got) != 1 || got[0] != "a/new" {
		t.Fatalf("les dépôts plus retrouvés depuis 30 j doivent disparaître : %v", got)
	}
}

func TestResultsRespectCreatedWithin(t *testing.T) {
	repos := []Repo{{FullName: "a/young", CreatedAt: t0.Add(-2 * day)}, {FullName: "a/aging", CreatedAt: t0.Add(-6 * day)}}
	fakeSearch(t, &repos)
	cfg := storeCfg(t)
	cfg.Watches[0].CreatedWithin = "7d"
	Fetch(cfg, "", t0)

	st := loadState(cfg.StateFile)
	if got := names(st.results(cfg, nil, t0.Add(3*day))[0].Items); len(got) != 1 || got[0] != "a/young" {
		t.Fatalf("un dépôt créé il y a plus de 7 j doit sortir de la veille : %v", got)
	}
}

func TestMarksPersistAndSurvivePruning(t *testing.T) {
	repos := []Repo{{FullName: "a/keep", Stars: 3}}
	fakeSearch(t, &repos)
	cfg := storeCfg(t)
	Fetch(cfg, "", t0)

	st := loadState(cfg.StateFile)
	if !st.toggle(Repo{FullName: "a/keep"}, markFav) {
		t.Fatal("toggle doit activer le favori")
	}
	st.toggle(Repo{FullName: "x/search", Stars: 9}, markLater) // dépôt hors veille
	if err := st.save(cfg.StateFile); err != nil {
		t.Fatal(err)
	}

	repos = nil
	Fetch(cfg, "", t0.Add(60*day)) // plus aucun résultat : élagage
	st = loadState(cfg.StateFile)
	if got := names(st.marked(markFav, day)); len(got) != 1 || got[0] != "a/keep" {
		t.Fatalf("favoris : %v", got)
	}
	if got := names(st.marked(markLater, day)); len(got) != 1 || got[0] != "x/search" {
		t.Fatalf("à voir : %v", got)
	}
	if fav, later := st.marks("a/keep"); !fav || later {
		t.Errorf("marks(a/keep) = %v, %v", fav, later)
	}
}

func TestMarkKeysInTUI(t *testing.T) {
	m := testModel(t)
	m.Update(key("f")) // a/two, sélectionné (500 ★)
	if got := names(m.tabs()[m.markTab(markFav)].Items); len(got) != 1 || got[0] != "a/two" {
		t.Fatalf("onglet favoris : %v", got)
	}
	m.Update(key("f"))
	if n := len(m.tabs()[m.markTab(markFav)].Items); n != 0 {
		t.Fatalf("« f » une deuxième fois doit retirer le favori (%d restant)", n)
	}
}

func TestFetchReadme(t *testing.T) {
	repos := []Repo{}
	fakeSearch(t, &repos)
	text, err := fetchReadme("o/r", "")
	if err != nil {
		t.Fatal(err)
	}
	if text != "Titre\n\nUn outil génial." {
		t.Fatalf("extrait : %q", text)
	}
	if text, err := fetchReadme("sans/readme", ""); err != nil || text != "" {
		t.Fatalf("sans README : %q, %v", text, err)
	}
}

func TestReadmeText(t *testing.T) {
	md := "<p align=\"center\"><img src=\"logo.png\"></p>\n\n" +
		"[![CI](https://x/ci.svg)](https://x) [![npm](https://x/n.svg)](https://x)\n\n" +
		"Project\n=======\n\n" +
		"<!-- caché -->\n" +
		"## Pourquoi ?\n\n" +
		"- rapide &amp; `simple`\n- voir [la doc][doc]\n\n" +
		"| a | b |\n|---|---|\n| 1 | 2 |\n\n" +
		"---\n\n[doc]: https://x/doc\n"
	want := "Project\n\nPourquoi ?\n\n• rapide & simple\n• voir la doc"
	if got := readmeText(md, 1000); got != want {
		t.Fatalf("got  %q\nwant %q", got, want)
	}
	long := strings.Repeat("mot ", 100)
	if got := readmeText(long, 50); len([]rune(got)) > 52 || !strings.HasSuffix(got, " …") {
		t.Fatalf("troncature : %q", got)
	}
}

// La description s'arrête avant la colonne des étoiles.
func TestListItemKeepsStatsColumnClear(t *testing.T) {
	m := testModel(t)
	it := Item{Repo: Repo{FullName: "owner/a-very-long-repository-name-indeed", Stars: 12345,
		Description: strings.Repeat("description très longue ", 10)}, Trend: Trend{OK: true, Gain: 1500}}
	const width = 70
	lines := strings.Split(m.viewItem(it, false, width), "\n")
	for i, l := range lines {
		plain := []rune(ansi.Strip(l))
		if len(plain) > width {
			t.Fatalf("ligne %d trop large (%d) : %q", i, len(plain), string(plain))
		}
		col := strings.TrimSpace(string(plain[width-statsCol:]))
		if want := []string{"★ 12k", "▲ +1.5k"}[i]; col != want {
			t.Errorf("ligne %d, colonne de droite = %q, attendu %q", i, col, want)
		}
	}
}
