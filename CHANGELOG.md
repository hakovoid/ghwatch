# Notes de version

Toutes les évolutions notables de ghwatch sont listées ici, de la plus
récente à la plus ancienne. Format inspiré de
[Keep a Changelog](https://keepachangelog.com/fr/1.1.0/), numérotation
[SemVer](https://semver.org/lang/fr/) (voir « Versions et releases » dans le
README).

## [0.4.0] — 2026-10-25

### Ajouté

- **Thèmes de couleurs**, les mêmes que vulnkb : violet, bleu, rose, vert,
  cyan, orange. `T` (ou `Ctrl+Y`) passe au suivant, `-theme nom` en choisit
  un, `GHWATCH_THEME` le force pour une session.
- Le thème et la disposition (détail à droite ou en dessous, partage
  liste / détail) sont mémorisés d'une session à l'autre.
- Veilles **cyber et OSINT** dans la config d'exemple : nouveautés cyber de
  la semaine, nouveautés OSINT du mois, outils cyber et OSINT populaires et
  actifs.
- README : comment trouver les topics d'un nouveau sujet et en faire une veille.

### Modifié

- La ligne sélectionnée a un fond teinté sur toute sa largeur.
- Le détail sépare le README par un titre de section ; la version est
  affichée dans l'en-tête.

### Corrigé

- Lignes en double ou écran décalé : les emoji des descriptions et des
  README (dont la largeur varie selon les terminaux) sont retirés à
  l'affichage, et aucune ligne ne peut plus dépasser la largeur du terminal
  (un seul élément trop large élargissait tout l'écran).

## [0.3.0] — 2026-10-24

### Ajouté

- Option `-version` : version, commit et date de compilation. La version est
  aussi rappelée dans l'aide de la TUI (`?`).
- Releases GitHub avec binaires prêts à l'emploi pour Linux (amd64, arm64),
  macOS (Intel, Apple Silicon) et Windows, et leurs sommes de contrôle.
- Script `scripts/release.sh` : publie une version en une commande (tests,
  tag, binaires, release avec les notes de ce fichier).

## [0.2.0] — 2026-10-24

### Ajouté

- **Mémoire locale** : les dépôts trouvés, leurs détails et la date de
  vérification de chaque veille sont enregistrés dans `ghwatch-state.json`.
  La TUI s'ouvre instantanément sur ces résultats et n'interroge GitHub que
  pour les veilles dont l'intervalle est écoulé.
- **Résultats cumulés** : chaque vérification ajoute les nouveaux dépôts à
  ceux déjà trouvés, au lieu de ne garder que le dernier passage. Nouveau
  réglage `keep` (30 jours par défaut) : durée de conservation d'un dépôt
  qu'aucune vérification ne retrouve.
- **Favoris et « à voir »** : `f` et `a` sur un dépôt, avec deux onglets
  dédiés ♥ Favoris et ◷ À voir. Les dépôts marqués ne sont jamais oubliés.
- **Extrait du README** dans le panneau de détail, téléchargé à la demande
  (quand un dépôt reste sélectionné) puis mémorisé 30 jours.
- Détail enrichi : site du projet, date de première découverte, mention
  « archivé ».
- **Disposition réglable** : détail sous la liste par défaut, `v` pour le
  placer à droite, `+` / `-` pour agrandir ou réduire la liste, `=` pour
  revenir au partage par défaut, `J` / `K` pour faire défiler le détail.
- Détail en deux colonnes sur les terminaux larges (texte à gauche, fiche
  à droite).
- Note `docs/token.md` : enregistrer le token GitHub dans un fichier protégé.

### Modifié

- Liste plus lisible : étoiles et progression dans une colonne fixe à
  droite, la description s'arrête avant ; badges NEW, ♥ et ◷ à côté du nom ;
  description atténuée hors sélection.
- Les noms d'onglets trop longs sont raccourcis (en gardant la partie après
  « · ») pour que tous les onglets restent visibles.
- `r` revérifie toutes les veilles, même si leur intervalle n'est pas écoulé.
- En-tête : la date de dernière vérification est affichée quand elle ne date
  pas du jour.
- Une veille en échec affiche ses résultats mémorisés avec un avertissement,
  au lieu d'un écran d'erreur.

### Notes de mise à jour

- Le premier lancement après la mise à jour vérifie toutes les veilles une
  fois, le temps de remplir la mémoire locale. Les dépôts déjà vus restent
  connus et ne sont pas marqués NEW à nouveau.
- Le mode texte (`./ghwatch`, `-once`) vérifie toujours toutes les veilles.

## [0.1.0] — 2026-10-24

Première version publiée.

### Ajouté

- Veilles configurables dans `config.json` : topics, langage, étoiles
  minimum, date de création ou de dernier commit, requête GitHub brute,
  termes combinés en OU (`any`), tri et limite.
- Intervalle de vérification configurable (`30m`, `6h`, `1d`, `1w`…), config
  relue à chaud.
- Mode texte en continu ou ponctuel (`-once`, pour cron), rapports Markdown
  dans `reports/`.
- Interface interactive (`-tui`) : onglets par veille, panneau de détail,
  filtre, tris, nouveautés seules, aide, rafraîchissement automatique.
- Recherche GitHub ponctuelle (`S`) avec historique, enregistrable comme
  veille (`w`).
- Historique des étoiles et progression (`trend_window`), tri par
  progression, mini-courbe.
- Liens cliquables (OSC 8), copie du lien (`c`), ouverture dans le navigateur.
- Respect des limites de l'API Search avec ou sans token.

[0.4.0]: https://github.com/hakovoid/ghwatch/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/hakovoid/ghwatch/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/hakovoid/ghwatch/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/hakovoid/ghwatch/releases/tag/v0.1.0
