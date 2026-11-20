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
    # Eino depends on Sonic, which does not support 32-bit ARM. Keep the
    # checked target and release target identical; publish phone arm64 only.
    MICROLIGHT_ANDROID_COMPRESS_LIBS=${MICROLIGHT_ANDROID_COMPRESS_LIBS:-1} bash scripts/build.sh -platform android/arm64
    package_slug=$(jq -er '.slug' mygo.json)
    package_version=$(jq -er '.version' mygo.json)
    apk="dist/android/$package_slug-$package_version.apk"
    [[ -f "$apk" ]] || { echo "Missing Android APK: $apk" >&2; exit 1; }
    # MyGo also generates an AAB; only stage the directly installable APK.
    if [[ ${GITHUB_REF_TYPE:-} == tag ]]; then
      # Verify both the APK signature and the identity CI was instructed to use.
      android_sdk=${ANDROID_HOME:-${ANDROID_SDK_ROOT:?Android SDK location is required}}
      signer="$android_sdk/build-tools/36.0.0/apksigner"
      "$signer" verify --verbose --print-certs "$apk" > "$signing_dir/apk-signature.txt"
      expected=$(keytool -exportcert -keystore "$signing_dir/upload.jks" \
        -storepass:env MYGO_ANDROID_KEYSTORE_PASSWORD -alias "$ANDROID_KEY_ALIAS" | openssl dgst -sha256 | awk '{print $NF}')
      actual=$(sed -n 's/^Signer #1 certificate SHA-256 digest: //p' "$signing_dir/apk-signature.txt")
      [[ "$actual" == "$expected" ]] || { echo 'APK signing certificate does not match the release key' >&2; exit 1; }
    fi
    cp "$apk" "dist/mobile/microlight-$version-android-arm64.apk"
    ;;
  *) echo "Unsupported mobile platform: $platform" >&2; exit 1 ;;
esac
