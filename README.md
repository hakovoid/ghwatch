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
| `s` | trier : étoiles → progression → plus récents → nom |
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

La config fournie surveille par défaut :

- **IA & agents · nouveautés de la semaine** : projets créés depuis moins de
  7 jours, ≥ 50 ★, qui ont un topic `llm`, `ai-agents`, `claude-code`, `mcp`,
  `agent-skills` ou `rag`, ou « claude », « agent » ou « llm » dans leur nom
  ou leur description.
- **IA & agents · ce qui monte ce mois-ci** : projets de moins de 30 jours
  déjà au-dessus de 500 ★ (ceux qui décollent, même s'ils ont raté la
  première semaine).
- **IA & agents · populaires et actifs** : projets IA établis (≥ 5 000 ★)
  avec un commit dans les 7 derniers jours, quelle que soit leur date de
  création. Triés par progression, ils montrent ce qui monte en ce moment.
- **Projets Go qui montent** et **Outils CLI récents**.
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
| `trend_window` | `"7d"` | Période de mesure de la progression des étoiles (défaut 7d) |
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
| `any` | `["topic:llm", "topic:mcp", "claude in:name,description"]` | Termes combinés en **OU** (voir ci-dessous) |
| `sort` | `"stars"` / `"forks"` / `"updated"` | Tri (défaut : stars) |
| `limit` | `10` | Nombre de résultats (max 100) |

### ET ou OU ?

Les critères d'une veille (`topics`, `language`, `min_stars`, `query`…) se
combinent en **ET** : `"topics": ["llm", "claude-code"]` exige les deux topics
à la fois. Or les topics sont choisis par les auteurs et souvent incomplets :
un projet d'agent IA peut n'avoir ni `llm` ni aucun topic.

Pour ratisser large, utilise **`any`** : chaque terme lance sa propre
recherche (avec les critères communs de la veille), puis les résultats sont
fusionnés, dédoublonnés, triés et limités à `limit`.

```json
{
  "name": "IA & agents · nouveautés de la semaine",
  "any": ["topic:llm", "topic:ai-agents", "topic:claude-code", "topic:mcp",
          "claude in:name,description", "agent in:name,description"],
  "created_within": "7d",
  "min_stars": 50,
  "limit": 15
}
```

Un terme peut être un topic (`topic:mcp`), un mot cherché dans le nom ou la
description (`claude in:name,description`), ou même dans le README
(`skill in:readme`, plus large mais plus bruyant).

Chaque terme compte comme une recherche : GitHub en autorise 30 par minute
avec un token et 10 sans. ghwatch espace les requêtes en conséquence
(2 s avec token, 6,5 s sans) : avec beaucoup de termes, configure un token.

### Progression des étoiles

L'API de recherche GitHub donne le nombre d'étoiles actuel, pas son
historique. ghwatch le construit lui-même : à chaque vérification, il
enregistre le nombre d'étoiles de chaque dépôt trouvé (dans le fichier
d'état, 90 jours conservés).

- **Dès la 2e vérification**, la TUI affiche le gain (« ▲ +1.8k ») dans la
  liste, et une mini-courbe des relevés dans le panneau de détail.
- Le gain est mesuré sur `trend_window` (7 jours par défaut). Tant que
  l'historique est plus court, il l'est depuis le premier relevé, et la
  période réelle est indiquée (« en 3 j »).
- `s` → **progression** trie par étoiles gagnées : c'est l'équivalent de
  GitHub Trending, limité aux dépôts de tes veilles.
- Un dépôt n'a d'historique que s'il est vérifié régulièrement : il doit
  rester dans les résultats d'une veille.

### Nouveaux projets ou projets établis ?

`created_within` ne garde que les projets **nés** récemment : un projet de
6 mois qui explose n'y apparaîtra jamais. Pour suivre les projets établis,
utilise plutôt `pushed_within` (activité récente) avec un seuil d'étoiles
élevé, et trie par progression. C'est le rôle de la veille
« IA & agents · populaires et actifs » de la config d'exemple.

## Lancer automatiquement

**En continu** (le plus simple) : `./ghwatch` dans un `tmux`, ou en service systemd.

**Avec cron**, en laissant cron gérer la fréquence :
```
0 8 * * * cd /chemin/ghwatch && GITHUB_TOKEN=ghp_xxx ./ghwatch -once >> ghwatch.log 2>&1
```

**Avec GitHub Actions** (gratuit, sans serveur) : un workflow planifié qui lance
`./ghwatch -once` et commit le dossier `reports/` et le fichier d'état.
