# ghwatch

Veille des dépôts GitHub récents ou populaires, par thème, langage et date,
en mode texte ou avec une interface interactive dans le terminal (TUI).

## Installation

```bash
go mod tidy                   # la première fois : télécharge et vérifie les dépendances
go build -o ghwatch .
export GITHUB_TOKEN=ghp_xxx   # recommandé : https://github.com/settings/tokens (aucun scope requis)
```

`go mod tidy` crée le fichier `go.sum` (sommes de contrôle des dépendances,
vérifiées par le serveur officiel de Go) : commite-le ensuite.

## Interface (TUI)

```bash
./ghwatch -tui
./ghwatch -tui -interval 6h
```

- Un onglet par veille, avec le nombre de nouveautés.
- Liste des dépôts : étoiles, langage, date de création, badge **NEW**.
- Panneau de détail à droite (terminal ≥ 100 colonnes) : description, forks,
  issues, licence, dernier commit, topics.
- Rafraîchissement automatique selon `interval`, compte à rebours en haut à droite.
- Thème qui s'adapte aux terminaux clairs et sombres.

| Touche | Action |
|---|---|
| `↑` `↓` / `j` `k` | dépôt précédent / suivant |
| `←` `→` / `h` `l` / `tab` | veille précédente / suivante |
| `Entrée` / `o` | ouvrir le dépôt dans le navigateur |
| `/` | filtrer (nom, description, langage, topic) — `esc` pour effacer |
| `s` | trier : étoiles → plus récents → nom |
| `n` | nouveautés uniquement / tout |
| `r` | actualiser maintenant (recharge la config) |
| `S` | recherche GitHub ponctuelle (voir ci-dessous) |
| `w` | depuis l'onglet de recherche : en faire une veille |
| `x` | fermer l'onglet de recherche |
| `?` | aide |
| `q` | quitter |

### Recherche GitHub

`S` ouvre une barre de saisie qui accepte toute la
[syntaxe de recherche GitHub](https://docs.github.com/fr/search-github/searching-on-github/searching-for-repositories) :

```
rag language:python stars:>100
topic:agents created:>2026-10-01
htmx in:name,description pushed:>2026-10-15
```

- Les 30 meilleurs résultats (par étoiles) s'ouvrent dans un onglet « ⌕ »,
  avec détail, tri, filtre et ouverture comme les veilles.
- Une recherche ne modifie ni tes veilles ni la liste des dépôts déjà vus.
- `w` l'ajoute à `config.json` comme nouvelle veille : elle sera alors
  vérifiée automatiquement à chaque intervalle.
- `↑` `↓` dans la barre de saisie parcourent l'historique des 30 dernières
  recherches (`ghwatch-history.json`).

## Ce que vérifie l'automatisme

À chaque intervalle, ghwatch relance **chaque veille de `config.json`**, et
seulement celles-ci, sur l'API de recherche GitHub. Il compare les résultats
aux dépôts déjà vus : ceux qui n'ont jamais été vus sont marqués **NEW**.
Pour changer ce qui est surveillé, modifie les veilles dans `config.json`
(relu à chaque passage) ou enregistre une recherche avec `w`.

## Mode texte

```bash
./ghwatch                 # tourne en continu, vérifie tous les "interval"
./ghwatch -interval 6h    # remplace la fréquence de la config
./ghwatch -once           # une vérification puis quitte (pour cron)
./ghwatch -all            # réaffiche aussi les dépôts déjà vus
./ghwatch -config x.json  # autre fichier de config
```

Chaque passage (texte ou TUI) affiche le rapport et l'enregistre dans `reports/AAAA-MM-JJ_HHhMM.md`.
Seuls les dépôts **jamais vus** sont listés (mémorisés dans `ghwatch-state.json`).
Pour repartir de zéro, supprime ce fichier.

## Configuration (`config.json`)

| Champ | Exemple | Rôle |
|---|---|---|
| `interval` | `"30m"`, `"6h"`, `"1d"`, `"1w"` | Fréquence des vérifications (minimum 1m) |
| `state_file` | `"ghwatch-state.json"` | Dépôts déjà vus |
| `reports_dir` | `"reports"` | Dossier des rapports (`""` = désactivé) |
| `watches` | liste | Tes veilles |

La config est relue à chaque passage : tu peux changer les thèmes ou la
fréquence sans redémarrer (la nouvelle fréquence s'applique au passage suivant).

### Critères d'une veille (tous optionnels, combinables)

| Champ | Exemple | Effet |
|---|---|---|
| `name` | `"IA de la semaine"` | Titre dans le rapport |
| `topics` | `["llm", "agents"]` | Topics GitHub |
| `language` | `"go"` | Langage principal |
| `min_stars` | `50` | Étoiles minimum |
| `created_within` | `"7d"` | Créés depuis moins de… |
| `pushed_within` | `"2d"` | Commit récent depuis moins de… |
| `query` | `"cli in:description"` | Syntaxe de recherche GitHub brute |
| `sort` | `"stars"` / `"forks"` / `"updated"` | Tri (défaut : stars) |
| `limit` | `10` | Nombre de résultats (max 100) |

## Lancer automatiquement

**En continu** (le plus simple) : `./ghwatch` dans un `tmux`, ou en service systemd.

**Avec cron**, en laissant cron gérer la fréquence :
```
0 8 * * * cd /chemin/ghwatch && GITHUB_TOKEN=ghp_xxx ./ghwatch -once >> ghwatch.log 2>&1
```

**Avec GitHub Actions** (gratuit, sans serveur) : un workflow planifié qui lance
`./ghwatch -once` et commit le dossier `reports/` et le fichier d'état.
