package main

import (
	"runtime/debug"
)

// version is stamped at build time (the Makefile and CI pass
// -ldflags "-X main.version=$(git describe --tags --always --dirty)").
var version string

// versionString is the stamped version, else what Go recorded in the binary
// (a module version, or the commit), else "dev".
func versionString() string { return resolveVersion(version, debug.ReadBuildInfo) }

func resolveVersion(stamped string, info func() (*debug.BuildInfo, bool)) string {
	if stamped != "" {
		return stamped
	}
	bi, ok := info()
	if !ok || bi == nil {
		return "dev"
	}
	if v := bi.Main.Version; v != "" && v != "(devel)" {
		return v
	}
	rev, dirty := "", false
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.modified":
			dirty = s.Value == "true"
		}
	}
	if len(rev) < 7 {
		return "dev"
	}
	v := "dev (" + rev[:7]
	if dirty {
		v += "+dirty"
	}
	return v + ")"
}
