#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.."

version=${MICROLIGHT_BUILD_VERSION:-}
if [[ -z "$version" ]]; then
  if version=$(git describe --tags --match 'v[0-9]*.[0-9]*.[0-9]*' --abbrev=7 HEAD 2>/dev/null); then
    if [[ "$version" =~ ^(.+)-[0-9]+-g([0-9a-f]+)$ ]]; then
      version="${BASH_REMATCH[1]}-${BASH_REMATCH[2]}"
    fi
  elif revision=$(git rev-parse --short=7 HEAD 2>/dev/null); then
    version="v0.0.0-$revision"
  else
    version=dev
  fi
  if changes=$(git status --porcelain=v1 --untracked-files=normal 2>/dev/null) && [[ -n "$changes" ]]; then
    version+=+dirty
  fi
fi

cli_version=$(go list -mod=readonly -m -f '{{.Version}}' github.com/egoist/mygo)
export CGO_ENABLED=0
export MYGO_LDFLAGS="${MYGO_LDFLAGS:+$MYGO_LDFLAGS }-X github.com/ggdream/microlight/internal/buildinfo.version=$version"
config_backup=
compression_tmp=
compression_init=
cleanup() {
  local status=$?
  trap - EXIT
  if [[ -n "$config_backup" ]]; then cp "$config_backup" mygo.json; rm -f "$config_backup"; fi
  if [[ -n "$compression_tmp" ]]; then rm -f "$compression_tmp" "$compression_init"; fi
  exit "$status"
}
trap cleanup EXIT
# MyGo 0.10.0 has no APK compression option. Apply AGP's public DSL through
# a temporary, project-scoped Gradle init script; never edit the SDK template.
case "${MICROLIGHT_ANDROID_COMPRESS_LIBS:-0}" in
  0) ;;
  1)
    gradle_home=${GRADLE_USER_HOME:-$HOME/.gradle}
    mkdir -p "$gradle_home/init.d"
    gradle_home=$(cd "$gradle_home" && pwd -P)
    export GRADLE_USER_HOME="$gradle_home"
    compression_tmp=$(mktemp "$gradle_home/init.d/microlight-apk.XXXXXX")
    compression_init="$compression_tmp.init.gradle"
    cat scripts/android/apk-compression.init.gradle > "$compression_tmp"
    mv "$compression_tmp" "$compression_init"
    export MICROLIGHT_ANDROID_BUILD_ROOT="$PWD/.mygo/android"
    ;;
  *) echo 'MICROLIGHT_ANDROID_COMPRESS_LIBS must be 0 or 1' >&2; exit 1 ;;
esac

original_args=("$@")
build_args=()
platforms=
while [[ $# -gt 0 ]]; do
  case "$1" in
    -platform|--platform) platforms=$2; shift 2 ;;
    -platform=*|--platform=*) platforms=${1#*=}; shift ;;
    *) build_args+=("$1"); shift ;;
  esac
done
if [[ -z "$platforms" ]]; then
  go run "github.com/egoist/mygo/cmd/mygo@$cli_version" build ${build_args[@]+"${build_args[@]}"}
else
  # MyGo has one shared icon setting. Select the full-bleed mobile asset only
  # for mobile targets; preserve desktop artwork even in a mixed-target build.
  IFS=, read -r -a targets <<< "$platforms"
  for target in "${targets[@]}"; do
    case "$target" in
      ios|ios/*|android|android/*)
        if [[ -z "$config_backup" ]]; then config_backup=$(mktemp); cp mygo.json "$config_backup"; fi
        jq '.icon = "assets/icon-mobile.png"' "$config_backup" > mygo.json
        ;;
      *) if [[ -n "$config_backup" ]]; then cp "$config_backup" mygo.json; fi ;;
    esac
    if [[ "$target" == web || "$target" == js/wasm ]]; then
      (
        touch_dir=$(mktemp -d)
        trap 'rm -rf "$touch_dir"' EXIT
        module_dir=$(go list -mod=readonly -m -f '{{.Dir}}' github.com/egoist/mygo)
        mkdir -p "$touch_dir/bin"
        GOBIN="$touch_dir/bin" go install "github.com/egoist/mygo/cmd/mygo@$cli_version"
        modfile=$(go run ./scripts/webtouch -module "$module_dir" -out "$touch_dir")
        export GOFLAGS="${GOFLAGS:+$GOFLAGS }-modfile=$modfile"
        "$touch_dir/bin/mygo" build ${build_args[@]+"${build_args[@]}"} -platform "$target"
      )
    else
      go run "github.com/egoist/mygo/cmd/mygo@$cli_version" build ${build_args[@]+"${build_args[@]}"} -platform "$target"
    fi
  done
  if [[ -n "$config_backup" ]]; then cp "$config_backup" mygo.json; fi
fi
bash scripts/web-assets.sh ${original_args[@]+"${original_args[@]}"}
