// edge's plugin for Ember (the adapter in package adapter): Ember runs it
// as `ember plugin edge` (ember install sets that up when edge is
// installed); built alone it is the same plugin as its own program.
package main

import (
	"log"

	"github.com/Tejas242/ember/pkg/plugin"
	"github.com/Tejas242/ember/plugins/edge/adapter"
)

func main() {
	if err := plugin.Run("edge", func(p *plugin.Plugin) error { return adapter.Run(p, adapter.Options{}) }); err != nil {
		log.Fatal(err)
	}
}
