# Enregistrer le token GitHub dans un fichier

ghwatch lit le token dans la variable `GITHUB_TOKEN`. Pour ne pas avoir à la
renseigner à chaque lancement, le lanceur `~/.local/bin/ghwatch` va le
chercher dans `~/.config/ghwatch/token` s'il n'est pas déjà défini.

Le token n'a besoin d'aucun droit : un token *fine-grained* avec
**Public repositories (read-only)** suffit
(https://github.com/settings/personal-access-tokens/new).

## Enregistrer le token

```bash
mkdir -p ~/.config/ghwatch
read -rs -p "Token : " t && printf '%s' "$t" > ~/.config/ghwatch/token && unset t
chmod 600 ~/.config/ghwatch/token
```

- `read -rs` : le token n'est ni affiché à l'écran ni enregistré dans
  l'historique bash.
- `unset t` : la variable est effacée de la session après l'écriture.
- `chmod 600` : le fichier n'est lisible que par toi.

Cette méthode est simple et marche partout, y compris en SSH ou sans session
graphique. Le token est stocké en clair, protégé uniquement par les droits
du fichier.

## Vérifier

```bash
ls -l ~/.config/ghwatch/token   # doit afficher -rw-------
ghwatch                         # lance la TUI avec le token
```

## Changer ou supprimer le token

Pour le remplacer, relance la commande `read` ci-dessus. Pour le supprimer :

```bash
rm ~/.config/ghwatch/token
```

Pense aussi à révoquer le token sur GitHub
(https://github.com/settings/personal-access-tokens) s'il n'est plus utilisé.
