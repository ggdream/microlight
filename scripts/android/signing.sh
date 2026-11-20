#!/usr/bin/env bash
# Keep the persistent Android signing identity outside the checkout.
set -euo pipefail
umask 077
signing_home=${MICROLIGHT_ANDROID_SIGNING_DIR:-$HOME/.config/microlight/android-signing}
repo=ggdream/microlight
case "${1:-}" in
  init)
    if [[ -e "$signing_home" ]]; then
      for file in release.jks keystore-password key-password key-alias; do
        [[ -s "$signing_home/$file" ]] || { echo "Incomplete signing identity in $signing_home; refusing to replace it" >&2; exit 1; }
      done
      echo "Reusing Android signing identity in $signing_home"
    else
      mkdir -p "$signing_home"
      chmod 700 "$signing_home"
      openssl rand -hex 32 > "$signing_home/keystore-password"
      cp "$signing_home/keystore-password" "$signing_home/key-password"
      printf 'microlight\n' > "$signing_home/key-alias"
      keytool -genkeypair -noprompt -storetype PKCS12 \
        -keystore "$signing_home/release.jks" \
        -storepass:file "$signing_home/keystore-password" \
        -keypass:file "$signing_home/key-password" \
        -alias microlight -keyalg RSA -keysize 3072 -sigalg SHA256withRSA \
        -validity 10000 -dname 'CN=Microlight, O=Microlight'
      echo "Created Android signing identity in $signing_home"
    fi
    keytool -exportcert -rfc -keystore "$signing_home/release.jks" \
      -storepass:file "$signing_home/keystore-password" \
      -alias "$(cat "$signing_home/key-alias")" -file "$signing_home/certificate.pem"
    chmod 600 "$signing_home"/*
    ;;
  upload)
    for file in release.jks keystore-password key-password key-alias; do
      [[ -s "$signing_home/$file" ]] || { echo 'Run bash scripts/android/signing.sh init first' >&2; exit 1; }
    done
    gh auth status --hostname github.com >/dev/null
    base64 < "$signing_home/release.jks" | tr -d '\r\n' | gh secret set ANDROID_KEYSTORE_BASE64 --repo "$repo"
    tr -d '\r\n' < "$signing_home/key-alias" | gh secret set ANDROID_KEY_ALIAS --repo "$repo"
    tr -d '\r\n' < "$signing_home/keystore-password" | gh secret set MYGO_ANDROID_KEYSTORE_PASSWORD --repo "$repo"
    tr -d '\r\n' < "$signing_home/key-password" | gh secret set MYGO_ANDROID_KEY_PASSWORD --repo "$repo"
    gh secret list --repo "$repo" --json name --jq '.[] | select(.name | test("^(ANDROID_|MYGO_ANDROID_)")) | .name'
    ;;
  *) echo 'Usage: bash scripts/android/signing.sh init|upload' >&2; exit 1 ;;
esac
