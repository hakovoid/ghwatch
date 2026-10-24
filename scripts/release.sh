#!/usr/bin/env bash
# Publie une version de ghwatch : tag git, binaires et release GitHub.
#
#   scripts/release.sh v0.3.0
#
# Prérequis : la section « ## [0.3.0] » existe dans CHANGELOG.md, l'arbre de
# travail est propre, on est sur main à jour, et gh est connecté.
set -euo pipefail

TAG="${1:-}"
if [[ ! "$TAG" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "usage : $0 vMAJEUR.MINEUR.CORRECTIF (ex : v0.3.0)" >&2
  exit 1
fi
VER="${TAG#v}"
cd "$(dirname "$0")/.."

die() { echo "erreur : $*" >&2; exit 1; }

[[ "$(git branch --show-current)" == main ]] || die "pas sur la branche main"
[[ -z "$(git status --porcelain)" ]] || die "modifications ou fichiers non suivis : le binaire serait marqué « modifié localement »"
git fetch -q origin main
[[ "$(git rev-parse HEAD)" == "$(git rev-parse origin/main)" ]] || die "main n'est pas à jour avec origin/main"
git rev-parse -q --verify "refs/tags/$TAG" >/dev/null && die "le tag $TAG existe déjà"

# notes de version : la section du CHANGELOG, sans son titre
NOTES="$(awk -v v="$VER" '
  $0 ~ "^## \\[" v "\\]" { on = 1; next }
  on && /^## \[/ { exit }
  on && /^\[[^]]+\]: / { exit }
  on { print }
' CHANGELOG.md)"
[[ -n "${NOTES//[[:space:]]/}" ]] || die "section « ## [$VER] » absente ou vide dans CHANGELOG.md"

echo "› tests"
go vet ./...
go test -count=1 ./... >/dev/null

echo "› tag $TAG"
git tag -a "$TAG" -m "$TAG"
git push -q origin "$TAG"

echo "› binaires"
DIST="dist/$TAG"
rm -rf "$DIST" && mkdir -p "$DIST"
for target in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64; do
  os="${target%/*}" arch="${target#*/}"
  name="ghwatch_${VER}_${os}_${arch}"
  bin="ghwatch"; [[ "$os" == windows ]] && bin="ghwatch.exe"
  mkdir -p "$DIST/$name"
  CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" \
    go build -trimpath -ldflags "-s -w -X main.version=$TAG" -o "$DIST/$name/$bin" .
  cp README.md CHANGELOG.md config.json "$DIST/$name/"
  if [[ "$os" == windows ]]; then
    (cd "$DIST" && zip -qr "$name.zip" "$name")
  else
    tar -C "$DIST" -czf "$DIST/$name.tar.gz" "$name"
  fi
  rm -rf "${DIST:?}/$name"
done
(cd "$DIST" && sha256sum ./*.tar.gz ./*.zip | sed 's| \./| |' > checksums.txt)

echo "› release GitHub"
gh release create "$TAG" "$DIST"/* --title "ghwatch $TAG" --notes "$NOTES" --verify-tag
echo "✓ $(gh release view "$TAG" --json url -q .url)"
