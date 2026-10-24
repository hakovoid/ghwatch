// ghwatch : veille des dépôts GitHub populaires / récents par thème.
//
// Usage :
//
//	ghwatch                      # tourne en continu, vérifie tous les `interval`
//	ghwatch -once                # une seule vérification puis quitte (pour cron)
//	ghwatch -config autre.json   # autre fichier de config
//	ghwatch -interval 6h         # remplace l'interval de la config
//	ghwatch -all                 # affiche aussi les dépôts déjà vus
//	ghwatch -tui                 # interface interactive dans le terminal
package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	cfgPath := flag.String("config", "config.json", "fichier de configuration")
	intervalFlag := flag.String("interval", "", "fréquence (ex: 30m, 6h, 1d, 1w) — remplace celle de la config")
	once := flag.Bool("once", false, "une seule vérification puis quitter")
	showAll := flag.Bool("all", false, "afficher aussi les dépôts déjà vus")
	tui := flag.Bool("tui", false, "interface interactive dans le terminal")
	flag.BoolVar(&linksEnabled, "links", true, "liens cliquables dans la TUI (désactiver si le terminal affiche des caractères parasites)")
	flag.Parse()

	if *tui {
		if err := runTUI(*cfgPath, *intervalFlag, os.Getenv("GITHUB_TOKEN")); err != nil {
			log.Fatal(err)
		}
		return
	}

	cfg, interval, err := loadConfigWithInterval(*cfgPath, *intervalFlag)
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		log.Println("GITHUB_TOKEN absent : limite de l'API réduite à 10 recherches/min, vérifications plus lentes")
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
			if c, d, err := loadConfigWithInterval(*cfgPath, *intervalFlag); err == nil {
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
