#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.."

version=$(git describe --tags --match 'v[0-9]*.[0-9]*.[0-9]*' --always HEAD 2>/dev/null || printf 'dev')
if [[ "$version" =~ ^(.+)-[0-9]+-g([0-9a-f]+)$ ]]; then
  version="${BASH_REMATCH[1]}-${BASH_REMATCH[2]}"
fi

cli_version=$(go list -mod=readonly -m -f '{{.Version}}' github.com/egoist/mygo)
export CGO_ENABLED=0
export MYGO_LDFLAGS="${MYGO_LDFLAGS:+$MYGO_LDFLAGS }-X github.com/ggdream/microlight/internal/buildinfo.version=$version"
exec go run "github.com/egoist/mygo/cmd/mygo@$cli_version" build "$@"
