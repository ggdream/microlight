#!/usr/bin/env bash
set -euo pipefail

all=false
macos=false
windows=false

if [[ "$GITHUB_EVENT_NAME" == workflow_dispatch || "$GITHUB_REF_TYPE" == tag ]]; then
  all=true
else
  if [[ "$GITHUB_EVENT_NAME" == pull_request ]]; then
    head=$(jq -er '.pull_request.head.sha' "$GITHUB_EVENT_PATH")
    target=$(jq -er '.pull_request.base.sha' "$GITHUB_EVENT_PATH")
    base=$(git merge-base "$target" "$head" || true)
  else
    base=$(jq -er '.before' "$GITHUB_EVENT_PATH")
    head=$(jq -er '.after' "$GITHUB_EVENT_PATH")
  fi

  if [[ -z "$base" ]] || ! git cat-file -e "$base^{commit}" 2>/dev/null; then
    # New branches and rewritten history can lack a comparable base.
    all=true
  else
    changes=$(mktemp)
    trap 'rm -f "$changes"' EXIT
    git diff --name-only --no-renames -z "$base" "$head" > "$changes"
    platform_suffix='_(darwin|linux|windows)(_(386|amd64|arm|arm64|loong64|mips|mips64|mips64le|mipsle|ppc64|ppc64le|riscv64|s390x|wasm))?(_test)?\.go$'
    while IFS= read -r -d '' path; do
      case "$path" in
        go.mod|go.sum|go.work|go.work.sum|mygo.json|mygo.config.ts|mygo-plugin.json|Makefile|.github/*|resources/*|scripts/*|build/*|assets/icon.*)
          all=true
          break
          ;;
      esac

      if [[ "${path##*/}" =~ $platform_suffix ]]; then
        case "${BASH_REMATCH[1]}" in
          darwin) macos=true ;;
          windows) windows=true ;;
        esac
        continue
      fi

      case "$path" in
        internal/pages/chat/view.go)
          macos=true
          ;;
        main.go|internal/app/*|internal/attachment/*|internal/desktop/*|internal/shell/*|internal/localtools/*|internal/workspace/*|pkg/proxy/*|pkg/a2uimygo/media.go|internal/pages/settings/proxy.go|internal/pages/chat/notifications.go|internal/pages/chat/conversation_access*|internal/pages/chat/proxy_settings*|internal/pages/chat/images*|internal/pages/chat/attachment_view.go|internal/pages/chat/composer/input.go|assets/tray.png)
          all=true
          break
          ;;
      esac

      if [[ "$path" == *.go ]]; then
        # Read both versions to account for files whose build tags changed.
        for revision in "$base" "$head"; do
          constraints=$(git show "$revision:$path" 2>/dev/null | sed -n '/^package[[:space:]]/q; /^\/\/go:build /p; /^\/\/ +build /p' || true)
          [[ -n "$constraints" ]] || continue
          case "$constraints" in
            *unix*|*'!'*) all=true ;;
          esac
          if [[ "$constraints" =~ (^|[^[:alnum:]_])darwin([^[:alnum:]_]|$) ]]; then macos=true; fi
          if [[ "$constraints" =~ (^|[^[:alnum:]_])windows([^[:alnum:]_]|$) ]]; then windows=true; fi
        done
      fi
    done < "$changes"
  fi
fi

matrix=$(jq -cn --argjson all "$all" --argjson macos "$macos" --argjson windows "$windows" '
  [{platform: "linux", runner: "ubuntu-24.04"}]
  + (if $all or $macos then [{platform: "darwin", runner: "macos-15"}] else [] end)
  + (if $all or $windows then [{platform: "windows", runner: "windows-2025"}] else [] end)
')
printf 'matrix=%s\n' "$matrix" >> "$GITHUB_OUTPUT"
printf 'Test matrix: %s\n' "$matrix"
