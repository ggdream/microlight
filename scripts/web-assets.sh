#!/usr/bin/env bash
# Stage the browser-only SQLite worker after a MyGo web production build.
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."
platforms=
output=
while [[ $# -gt 0 ]]; do
  case "$1" in
    -platform|--platform) platforms=$2; shift 2 ;;
    -platform=*|--platform=*) platforms=${1#*=}; shift ;;
    -o|--o) output=$2; shift 2 ;;
    -o=*|--o=*) output=${1#*=}; shift ;;
    *) shift ;;
  esac
done
case ",$platforms," in
  *,web,*|*,js/wasm,*) ;;
  *) exit 0 ;;
esac
if [[ -z "$output" ]]; then output=$(jq -r '.out // "dist"' mygo.json); fi
(
  cd pkg/sqliteweb/web/vendor
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum --check SHA256SUMS >/dev/null
  else
    shasum -a 256 --check SHA256SUMS >/dev/null
  fi
)
mkdir -p "$output/web/sqlite"
cp -R pkg/sqliteweb/web/. "$output/web/sqlite/"
cp pkg/webnotify/web/notification-worker.js "$output/web/notification-worker.js"
