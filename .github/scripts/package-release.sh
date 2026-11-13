#!/usr/bin/env bash
set -euo pipefail
export CGO_ENABLED=0

tag=$GITHUB_REF_NAME
if [[ ! "$tag" =~ ^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]]; then
  printf 'Expected a release tag in the form vx.x.x, got %s\n' "$tag" >&2
  exit 1
fi

target="$RELEASE_PLATFORM-$RELEASE_ARCH"
case "$target" in
  darwin-universal) extensions=(dmg) ;;
  linux-amd64) extensions=(deb rpm tar.gz) ;;
  windows-amd64) extensions=(exe) ;;
  *) printf 'Unsupported release target %s\n' "$target" >&2; exit 1 ;;
esac

cli_version=$(go list -mod=readonly -m -f '{{.Version}}' github.com/egoist/mygo)
backup=$(mktemp)
cp mygo.json "$backup"
trap 'cp "$backup" mygo.json && rm -f "$backup"' EXIT
jq --arg version "${tag#v}" '.version = $version' "$backup" > mygo.json

go run "github.com/egoist/mygo/cmd/mygo@$cli_version" build \
  -platform "$RELEASE_PLATFORM/$RELEASE_ARCH" -o dist

shopt -s nullglob
for extension in "${extensions[@]}"; do
  if [[ "$RELEASE_PLATFORM" == windows ]]; then
    matches=(dist/"$target"/*-setup-*."$extension")
  else
    matches=(dist/"$target"/*."$extension")
  fi
  if [[ ${#matches[@]} != 1 ]]; then
    printf 'Expected one %s package in dist/%s, found %s\n' "$extension" "$target" "${#matches[@]}" >&2
    exit 1
  fi
  asset="dist/microlight-$tag-$target.$extension"
  cp "${matches[0]}" "$asset"
  printf '%s\n' "$asset"
done
