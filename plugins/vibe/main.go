// The vibe plugin for Ember (package vibe): Ember runs it as `ember plugin
// vibe` (ember install sets that up); built alone it is the same plugin as
// its own program.
package main

import (
	"log"

	"github.com/Tejas242/ember/pkg/plugin"
	"github.com/Tejas242/ember/plugins/vibe/vibe"
)

func main() {
	if err := plugin.Run("vibe", func(p *plugin.Plugin) error { return vibe.Run(p, vibe.Options{}) }); err != nil {
		log.Fatal(err)
	}
}
