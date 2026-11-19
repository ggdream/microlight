#!/usr/bin/env bash
set -euo pipefail

platform=${1:?Expected ios or android}
backup=$(mktemp)
cp mygo.json "$backup"
signing_dir=
keychain=
cleanup() {
  cp "$backup" mygo.json
  rm -f "$backup"
  if [[ -n "$keychain" ]]; then security delete-keychain "$keychain" || true; fi
  if [[ -n "$signing_dir" ]]; then rm -rf "$signing_dir"; fi
}
trap cleanup EXIT
version=${GITHUB_REF_NAME:-dev}
if [[ ${GITHUB_REF_TYPE:-} == tag ]]; then
  jq --arg version "${version#v}" '.version = $version' "$backup" > mygo.json
else
  version=$(git describe --tags --always --dirty)
fi
export MICROLIGHT_BUILD_VERSION=$version
mkdir -p dist/mobile
build_signed_ios=false
android_signing=debug
if [[ ${GITHUB_REF_TYPE:-} == tag ]]; then source .github/scripts/sign-mobile.sh; fi
case "$platform" in
  ios)
    # MyGo's simulator target is arm64; use the standard Apple Silicon runner.
    [[ $(uname -m) == arm64 ]] || { echo 'iOS simulator requires an arm64 Mac' >&2; exit 1; }
    bash scripts/build.sh -platform ios/simulator
    ditto -c -k --sequesterRsrc --keepParent dist/ios-simulator/Microlight.app \
      "dist/mobile/microlight-$version-ios-simulator-arm64.zip"
    if [[ "$build_signed_ios" == true ]]; then
      bash scripts/build.sh -platform ios
      files=(dist/ios-arm64/*.ipa)
      [[ ${#files[@]} == 1 && -f ${files[0]} ]] || { echo 'Expected one signed IPA' >&2; exit 1; }
      cp "${files[0]}" "dist/mobile/microlight-$version-ios-arm64.ipa"
    fi
    ;;
  android)
    target=android/arm64
    if [[ ${GITHUB_REF_TYPE:-} == tag ]]; then target=android; fi
    bash scripts/build.sh -platform "$target"
    files=(dist/android/*.apk)
    [[ ${#files[@]} == 1 && -f ${files[0]} ]] || { echo 'Expected one Android APK' >&2; exit 1; }
    # MyGo also generates an AAB; only stage the directly installable APK.
    # Without an upload keystore MyGo uses its debug key: label that honestly.
    cp "${files[0]}" "dist/mobile/microlight-$version-android-$android_signing.apk"
    ;;
  *) echo "Unsupported mobile platform: $platform" >&2; exit 1 ;;
esac
