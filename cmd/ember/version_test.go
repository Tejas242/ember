package main

import (
	"runtime/debug"
	"testing"
)

func TestResolveVersion(t *testing.T) {
	info := func(main string, settings ...string) func() (*debug.BuildInfo, bool) {
		return func() (*debug.BuildInfo, bool) {
			bi := &debug.BuildInfo{Main: debug.Module{Version: main}}
			for i := 0; i+1 < len(settings); i += 2 {
				bi.Settings = append(bi.Settings, debug.BuildSetting{Key: settings[i], Value: settings[i+1]})
			}
			return bi, true
		}
	}
	none := func() (*debug.BuildInfo, bool) { return nil, false }
	for _, tc := range []struct {
		stamped string
		info    func() (*debug.BuildInfo, bool)
		want    string
	}{
		{"v1.2.0-3-gabc1234-dirty", info("v9"), "v1.2.0-3-gabc1234-dirty"},
		{"", info("v1.1.0"), "v1.1.0"},
		{"", info("(devel)", "vcs.revision", "e8b140f0123", "vcs.modified", "true"), "dev (e8b140f+dirty)"},
		{"", info("(devel)", "vcs.revision", "e8b140f0123"), "dev (e8b140f)"},
		{"", info("(devel)"), "dev"},
		{"", none, "dev"},
	} {
		if got := resolveVersion(tc.stamped, tc.info); got != tc.want {
			t.Errorf("resolveVersion(%q) = %q, want %q", tc.stamped, got, tc.want)
		}
	}
}

func TestSameVersion(t *testing.T) {
	if ok, _ := sameVersion("v1.2.0", "v1.2.0"); !ok {
		t.Fatal("same build flagged")
	}
	if ok, d := sameVersion("v1.1.0", "v1.2.0"); ok || d != "the island runs v1.1.0, this binary is v1.2.0: restart it" {
		t.Fatal(d)
	}
	if ok, d := sameVersion("", "v1.2.0"); ok || d == "" {
		t.Fatal("an island without a version must be flagged")
	}
}
