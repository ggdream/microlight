// Package buildinfo reports the version embedded in the executable.
package buildinfo

import "runtime/debug"

// version is set by the MyGo build script through the Go linker.
var version string

func Version() string {
	if version != "" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		if info.Main.Version != "" && info.Main.Version != "(devel)" {
			return info.Main.Version
		}
		for _, setting := range info.Settings {
			if setting.Key == "vcs.revision" {
				revision := setting.Value
				return revision[:min(len(revision), 7)]
			}
		}
	}
	return "dev"
}
