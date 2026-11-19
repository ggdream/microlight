#!/usr/bin/env bash
# Build the static website and five standalone, asset-embedded HTTP servers.
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."
export CGO_ENABLED=0
version=${MICROLIGHT_BUILD_VERSION:-$(git describe --tags --always --dirty)}
if [[ ! "$version" =~ ^[a-zA-Z0-9.+-]+$ ]]; then echo 'Invalid package version' >&2; exit 1; fi
stage=$(mktemp -d)
assets="$PWD/cmd/webserver/site"
cleanup() {
  rm -rf "$stage"
  find "$assets" -mindepth 1 ! -name .keep -exec rm -rf {} +
}
trap cleanup EXIT
if [[ -n "${MICROLIGHT_WEB_SITE:-}" ]]; then
  cp -R "$MICROLIGHT_WEB_SITE/." "$stage/site"
else
  MICROLIGHT_BUILD_VERSION="$version" bash scripts/build.sh -platform web -o "$stage/product"
  mv "$stage/product/web" "$stage/site"
fi
for required in index.html wasm_exec.js sqlite/bridge.js sqlite/worker.js sqlite/vendor/dist/wa-sqlite-async.wasm; do
  test -f "$stage/site/$required" || { echo "Missing web asset: $required" >&2; exit 1; }
done
# Root-relative asset resolution also works after reloading a History route.
awk '{sub(/<head>/, "<head>\n    <base href=\"/\" />"); print}' "$stage/site/index.html" > "$stage/index.html"
mv "$stage/index.html" "$stage/site/index.html"
mkdir -p dist "$stage/servers"
tar -czf "dist/microlight-$version-web-static.tar.gz" -C "$stage/site" .
cp -R "$stage/site/." "$assets/"
for target in windows/amd64 linux/amd64 linux/arm64 darwin/amd64 darwin/arm64; do
  platform=${target%/*}; arch=${target#*/}
  name="microlight-web-$platform-$arch"
  if [[ "$platform" == windows ]]; then name+=.exe; fi
  GOOS="$platform" GOARCH="$arch" go build -mod=readonly -trimpath -ldflags='-s -w' -o "$stage/servers/$name" ./cmd/webserver
 done
cp LICENSE "$stage/servers/LICENSE"
cat > "$stage/servers/README.txt" <<'TEXT'
Run the binary for your OS/architecture: ./microlight-web-linux-amd64 -addr :8080
Windows: microlight-web-windows-amd64.exe -addr :8080
Open http://localhost:8080/ or http://SERVER_IP:8080/ from another device.
All web assets are embedded. No separate site directory is required.
For remote production access, put HTTPS in front of this server.
Browser data is local to each browser origin. Model endpoints must allow CORS.
TEXT
(cd "$stage/servers" && if command -v sha256sum >/dev/null 2>&1; then sha256sum microlight-web-* > SHA256SUMS; else shasum -a 256 microlight-web-* > SHA256SUMS; fi)
tar -czf "dist/microlight-$version-web-servers.tar.gz" -C "$stage/servers" .
printf 'Web packages: dist/microlight-%s-web-{static,servers}.tar.gz\n' "$version"
