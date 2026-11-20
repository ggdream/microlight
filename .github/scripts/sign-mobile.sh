#!/usr/bin/env bash
# Sourced by build-mobile.sh only on release tags. Never print signing values.
set -euo pipefail
case "$platform" in
  ios)
    if [[ -z ${IOS_CERTIFICATE_BASE64:-} && -z ${IOS_PROFILE_BASE64:-} && -z ${IOS_CERTIFICATE_PASSWORD:-} ]]; then
      echo '::notice::Apple signing is not configured; publishing the iOS simulator archive only.'
      return
    fi
    : "${IOS_CERTIFICATE_BASE64:?Missing iOS certificate}"
    : "${IOS_CERTIFICATE_PASSWORD:?Missing iOS certificate password}"
    : "${IOS_PROFILE_BASE64:?Missing iOS provisioning profile}"
    signing_dir=$(mktemp -d)
    keychain="$signing_dir/signing.keychain-db"
    keychain_password=$(openssl rand -hex 24)
    printf '%s' "$IOS_CERTIFICATE_BASE64" | base64 --decode > "$signing_dir/certificate.p12"
    printf '%s' "$IOS_PROFILE_BASE64" | base64 --decode > "$signing_dir/profile.mobileprovision"
    security create-keychain -p "$keychain_password" "$keychain"
    security set-keychain-settings -lut 21600 "$keychain"
    security unlock-keychain -p "$keychain_password" "$keychain"
    security import "$signing_dir/certificate.p12" -P "$IOS_CERTIFICATE_PASSWORD" -A -t cert -f pkcs12 -k "$keychain" >/dev/null
    security set-key-partition-list -S apple-tool:,apple:,codesign: -k "$keychain_password" "$keychain" >/dev/null
    security list-keychains -d user -s "$keychain" "$HOME/Library/Keychains/login.keychain-db"
    jq --arg profile "$signing_dir/profile.mobileprovision" '.ios.provisioningProfile = $profile' mygo.json > "$signing_dir/config.json"
    cp "$signing_dir/config.json" mygo.json
    build_signed_ios=true
    ;;
  android)
    : "${ANDROID_KEYSTORE_BASE64:?Missing Android keystore}"
    : "${ANDROID_KEY_ALIAS:?Missing Android key alias}"
    : "${MYGO_ANDROID_KEYSTORE_PASSWORD:?Missing Android keystore password}"
    : "${MYGO_ANDROID_KEY_PASSWORD:?Missing Android key password}"
    umask 077
    signing_dir=$(mktemp -d)
    printf '%s' "$ANDROID_KEYSTORE_BASE64" | base64 --decode > "$signing_dir/upload.jks"
    jq --arg path "$signing_dir/upload.jks" --arg alias "$ANDROID_KEY_ALIAS" \
      '.android.keystore = $path | .android.keyAlias = $alias' mygo.json > "$signing_dir/config.json"
    cp "$signing_dir/config.json" mygo.json
    ;;
esac
