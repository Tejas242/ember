package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Tejas242/ember/internal/xdg"
)

// vibeManifest is vibe's plugin.toml, running this binary.
func vibeManifest(exe string) string {
	return fmt.Sprintf(`# vibe, on Ember's island: written by ember install.
name         = "vibe"
description  = "Click Ember while a song plays: a stage, with the song's words"
command      = %q
args         = ["plugin", "vibe"]
icon         = "headphones"
capabilities = ["dashboard"]
grants       = ["media", "entry"]

[opens]
ember = "music"
hint  = "Vibe"
`, exe)
}

// installVibe sets up vibe's plugin: its manifest, and the grants it needs
// (the song while its stage is open, a click on Ember to open it), unless the
// config already says something about them.
func installVibe(exe string) error {
	dir := filepath.Join(pluginsDir(), "vibe")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := xdg.AtomicWrite(filepath.Join(dir, "plugin.toml"), []byte(vibeManifest(exe)), 0o644); err != nil {
		return err
	}
	fmt.Println("vibe →", filepath.Join(dir, "plugin.toml"))
	return grantOnce("vibe", `["media", "entry"]`,
		"vibe: click Ember in the card while a song plays. media: it hears what plays, only\n# while its stage is open; entry: the click on Ember opens it. Take one out to take it back.")
}
