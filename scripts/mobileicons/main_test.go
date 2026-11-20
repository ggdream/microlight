package main

import (
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func readIcon(t *testing.T, name string) image.Image {
	t.Helper()
	file, err := os.Open(filepath.Join("..", "..", "assets", name))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	img, err := png.Decode(file)
	if err != nil {
		t.Fatal(err)
	}
	return img
}

func TestMobileIconAssetsFillLauncherMasks(t *testing.T) {
	mobile := readIcon(t, "icon-mobile.png")
	adaptive := readIcon(t, "icon-android.png")
	for _, tc := range []struct {
		img  image.Image
		size int
	}{{mobile, 1024}, {adaptive, 1536}} {
		if tc.img.Bounds() != image.Rect(0, 0, tc.size, tc.size) {
			t.Fatalf("invalid icon size: %v", tc.img.Bounds())
		}
		for y := 0; y < tc.size; y++ {
			for x := 0; x < tc.size; x++ {
				_, _, _, a := tc.img.At(x, y).RGBA()
				if a != 65535 {
					t.Fatalf("transparent launcher pixel at %d,%d", x, y)
				}
			}
		}
	}
	// Android's 72/108 viewport must show exactly the iOS/legacy master.
	for y := 0; y < 1024; y++ {
		for x := 0; x < 1024; x++ {
			if mobile.At(x, y) != adaptive.At(x+256, y+256) {
				t.Fatalf("adaptive viewport changed the logo at %d,%d", x, y)
			}
		}
	}
	// The original desktop icon still has transparent margins.
	_, _, _, alpha := readIcon(t, "icon.png").At(0, 0).RGBA()
	if alpha != 0 {
		t.Fatal("desktop icon lost its transparent margin")
	}
}

func TestBuildSelectsMobileIconAndRestoresConfig(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell packaging is exercised on Unix runners")
	}
	if _, err := exec.LookPath("jq"); err != nil {
		t.Skip("jq is required for packaging")
	}
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "mixed targets", true: "failed build"}[fail], func(t *testing.T) {
			dir := t.TempDir()
			scripts := filepath.Join(dir, "scripts")
			bin := filepath.Join(dir, "bin")
			for _, path := range []string{scripts, bin} {
				if err := os.MkdirAll(path, 0700); err != nil {
					t.Fatal(err)
				}
			}
			for _, name := range []string{"build.sh", "web-assets.sh"} {
				data, err := os.ReadFile(filepath.Join("..", name))
				if err != nil {
					t.Fatal(err)
				}
				if err = os.WriteFile(filepath.Join(scripts, name), data, 0700); err != nil {
					t.Fatal(err)
				}
			}
			original := []byte("{\"icon\":\"assets/icon.png\"}\n")
			config := filepath.Join(dir, "mygo.json")
			if err := os.WriteFile(config, original, 0600); err != nil {
				t.Fatal(err)
			}
			stub := `#!/usr/bin/env bash
set -eu
if [[ "$1" == list ]]; then echo v0.test; exit 0; fi
platform=
while [[ $# -gt 0 ]]; do
 if [[ "$1" == -platform ]]; then platform=$2; shift 2; else shift; fi
done
printf '%s %s\n' "$platform" "$(jq -r .icon mygo.json)" >> "$MICROLIGHT_ICON_TEST_LOG"
if [[ "$MICROLIGHT_ICON_TEST_FAIL" == 1 ]]; then exit 7; fi
`
			if err := os.WriteFile(filepath.Join(bin, "go"), []byte(stub), 0700); err != nil {
				t.Fatal(err)
			}
			failValue := "0"
			if fail {
				failValue = "1"
			}
			log := filepath.Join(dir, "build.log")
			cmd := exec.Command("bash", filepath.Join(scripts, "build.sh"), "-platform", "android/arm64,ios/simulator,darwin/arm64")
			cmd.Env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"), "MICROLIGHT_BUILD_VERSION=test", "MICROLIGHT_ANDROID_COMPRESS_LIBS=0", "MICROLIGHT_ICON_TEST_LOG="+log, "MICROLIGHT_ICON_TEST_FAIL="+failValue)
			output, err := cmd.CombinedOutput()
			if (err != nil) != fail {
				t.Fatalf("unexpected build result: %v %s", err, output)
			}
			restored, err := os.ReadFile(config)
			if err != nil || string(restored) != string(original) {
				t.Fatalf("configuration not restored: %s %v", restored, err)
			}
			got, err := os.ReadFile(log)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(got), "android/arm64 assets/icon-mobile.png") {
				t.Fatalf("mobile icon not selected: %s", got)
			}
			if !fail && string(got) != "android/arm64 assets/icon-mobile.png\nios/simulator assets/icon-mobile.png\ndarwin/arm64 assets/icon.png\n" {
				t.Fatalf("mixed target icons: %s", got)
			}
		})
	}
}
