# ghwatch

Veille des dépôts GitHub récents ou populaires, par thème, langage et date.
Aucune dépendance : seulement la bibliothèque standard de Go.

## Installation

```bash
go build -o ghwatch .
export GITHUB_TOKEN=ghp_xxx   # recommandé : https://github.com/settings/tokens (aucun scope requis)
```

## Utilisation

```bash
./ghwatch                 # tourne en continu, vérifie tous les "interval"
./ghwatch -interval 6h    # remplace la fréquence de la config
./ghwatch -once           # une vérification puis quitte (pour cron)
./ghwatch -all            # réaffiche aussi les dépôts déjà vus
./ghwatch -config x.json  # autre fichier de config
```

Chaque passage affiche le rapport et l'enregistre dans `reports/AAAA-MM-JJ_HHhMM.md`.
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
