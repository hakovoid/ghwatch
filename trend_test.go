package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

var t0 = time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC)

const day = 24 * time.Hour

func TestRecordStarsMergesAndPrunes(t *testing.T) {
	s := loadState(t.TempDir() + "/absent.json")
	s.recordStars("a/b", 10, t0)
	s.recordStars("a/b", 12, t0.Add(20*time.Minute)) // < 1 h : remplace
	if pts := s.Stars["a/b"]; len(pts) != 1 || pts[0].N != 12 {
		t.Fatalf("relevés rapprochés non fusionnés : %+v", pts)
	}
	s.recordStars("a/b", 20, t0.Add(day))
	s.recordStars("a/b", 90, t0.Add(100*day)) // les relevés de plus de 90 j sont oubliés
	if pts := s.Stars["a/b"]; len(pts) != 1 || pts[0].N != 90 {
		t.Fatalf("historique non élagué : %+v", pts)
	}
}

func TestTrendWindow(t *testing.T) {
	s := loadState(t.TempDir() + "/absent.json")
	week := 7 * day

	if tr := s.trend("a/b", 100, t0, week); tr.OK {
		t.Fatal("aucun historique : la progression ne doit pas être mesurée")
	}
	s.recordStars("a/b", 100, t0)
	if tr := s.trend("a/b", 105, t0.Add(10*time.Minute), week); tr.OK {
		t.Fatal("un relevé vieux de 10 min ne suffit pas")
	}

	// Historique plus court que la fenêtre : référence = plus ancien relevé.
	s.recordStars("a/b", 300, t0.Add(2*day))
	tr := s.trend("a/b", 400, t0.Add(3*day), week)
	if !tr.OK || tr.Gain != 300 || tr.Span != 3*day {
		t.Fatalf("got %+v, attendu +300 sur 3 j", tr)
	}
	if len(tr.Points) != 3 || tr.Points[2].N != 400 {
		t.Errorf("les points doivent inclure le relevé courant : %+v", tr.Points)
	}

	// Historique plus long : référence = dernier relevé d'au moins 7 j.
	s.recordStars("a/b", 500, t0.Add(5*day))
	tr = s.trend("a/b", 1000, t0.Add(9*day), week)
	if !tr.OK || tr.Gain != 700 || tr.Span != 7*day {
		t.Fatalf("got %+v, attendu +700 sur 7 j (depuis le relevé à J+2)", tr)
	}
}

func TestFetchRecordsTrendAcrossRuns(t *testing.T) {
	stars := 100
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(searchResp{Items: []Repo{{FullName: "m/rea", Stars: stars}}})
	}))
	defer srv.Close()
	t.Setenv("GITHUB_API_URL", srv.URL)
	searchPause, searchPauseAnon = 0, 0

	cfg := &Config{StateFile: t.TempDir() + "/s.json", Watches: []Watch{{Name: "x", Language: "go", Limit: 10}}}
	if it := Fetch(cfg, "", t0)[0].Items[0]; it.Trend.OK {
		t.Fatal("1er passage : pas encore de progression")
	}
	stars = 340
	it := Fetch(cfg, "", t0.Add(2*day))[0].Items[0]
	if !it.Trend.OK || it.Trend.Gain != 240 {
		t.Fatalf("2e passage : %+v, attendu +240", it.Trend)
	}
	if pts := loadState(cfg.StateFile).Stars["m/rea"]; len(pts) != 2 {
		t.Fatalf("relevés enregistrés : %+v", pts)
	}
}

func TestTrendWindowConfig(t *testing.T) {
	if d := (&Config{}).trendWindow(); d != 7*day {
		t.Errorf("défaut = %v, attendu 7 j", d)
	}
	if d := (&Config{TrendWindow: "1d"}).trendWindow(); d != day {
		t.Errorf("1d = %v", d)
	}
}

func TestSparklineAndLabels(t *testing.T) {
	pts := []StarPoint{{N: 10}, {N: 20}, {N: 30}, {N: 80}}
	if s := sparkline(pts, 10); s != "▁▂▃█" {
		t.Errorf("sparkline = %q", s)
	}
	if s := sparkline(pts[:1], 10); s != "" {
		t.Errorf("un seul point : %q", s)
	}
	if s := sparkline([]StarPoint{{N: 5}, {N: 5}}, 10); s != "▁▁" {
		t.Errorf("valeurs égales : %q", s)
	}
	if got := humanGain(1240); got != "+1.2k" {
		t.Errorf("humanGain = %q", got)
	}
	if got := spanLabel(7*day - time.Hour); got != "7 j" {
		t.Errorf("spanLabel = %q", got)
	}
}

func TestSortByTrend(t *testing.T) {
	m := testModel(t)
	items := m.results[0].Items
	items[0].Trend = Trend{OK: true, Gain: 900} // a/one, 10 ★
	items[1].Trend = Trend{OK: false}           // a/two, 500 ★ : inconnue → fin
	items[2].Trend = Trend{OK: true, Gain: 50}  // a/three
	typeText(m, "s")
	if got := names(m.visible()); got[0] != "a/one" || got[2] != "a/two" {
		t.Fatalf("tri par progression : %v", got)
	}
}
