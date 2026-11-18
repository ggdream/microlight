#!/usr/bin/env bash
set -euo pipefail
export CGO_ENABLED=0

tag=$GITHUB_REF_NAME
if [[ ! "$tag" =~ ^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]]; then
  printf 'Expected a release tag in the form vx.x.x, got %s\n' "$tag" >&2
  exit 1
fi
# Capture the release tag before generating the packaging configuration.
version=$(git describe --tags --exact-match --match "$tag" HEAD)

target="$RELEASE_PLATFORM-$RELEASE_ARCH"
case "$target" in
  darwin-universal) extensions=(dmg) ;;
  linux-amd64) extensions=(deb rpm tar.gz) ;;
  windows-amd64) extensions=(exe) ;;
  *) printf 'Unsupported release target %s\n' "$target" >&2; exit 1 ;;
esac

backup=$(mktemp)
cp mygo.json "$backup"
trap 'cp "$backup" mygo.json && rm -f "$backup"' EXIT
public_key=${MYGO_UPDATER_PUBLIC_KEY:-}
if [[ -n "$public_key" || -n "${MYGO_UPDATER_PRIVATE_KEY:-}" ]]; then
  if [[ -z "$public_key" || -z "${MYGO_UPDATER_PRIVATE_KEY:-}" ]]; then
    printf 'Signed updates require both updater keys\n' >&2
    exit 1
  fi
fi
jq --arg version "${tag#v}" --arg key "$public_key" '
  .version = $version |
  if $key != "" then .updates = {publicKey: $key, github: "ggdream/microlight", deltas: 0} else . end
' "$backup" > mygo.json

export MYGO_LDFLAGS="${MYGO_LDFLAGS:+$MYGO_LDFLAGS }-X github.com/ggdream/microlight/internal/pages/chat.updateTarget=$target"

MICROLIGHT_BUILD_VERSION="$version" bash scripts/build.sh \
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

if [[ -n "$public_key" ]]; then
  manifest="update-$target.json"
  archive="microlight-${tag#v}-$target.tar.gz"
  cp "dist/$target/$manifest" "dist/$target/$archive" dist/
fi
