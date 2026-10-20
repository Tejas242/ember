//go:build render

package island

import (
	"encoding/json"
	"os"

	proto "github.com/Tejas242/ember/pkg/plugin"
	"github.com/Tejas242/ember/plugins/edge/adapter"
)

// edgeCard shows one of edge's cards the way the adapter and the host would.
func (a *app) edgeCard(c *adapter.Card) {
	pc, _ := adapter.Cardify(c)
	b, _ := proto.Line("card", pc)
	m, err := proto.Decode(b)
	if err != nil {
		panic(err)
	}
	a.applyExt("edge", m)
}

// edgeFixture is the adapter's sample stats: a few days of practice.
func edgeFixture() *adapter.Stats {
	b, err := os.ReadFile("../../plugins/edge/adapter/testdata/stats.json")
	if err != nil {
		panic(err)
	}
	var st adapter.Stats
	if err := json.Unmarshal(b, &st); err != nil {
		panic(err)
	}
	return &st
}

// edgeDriver plays edge's socket messages the way its plugin does, straight
// into the island (the host's limits aside).
type edgeDriver struct {
	a      *app
	status *adapter.Status
	stats  *adapter.Stats
}

func (d *edgeDriver) say(raw []byte) {
	var m adapter.Message
	if json.Unmarshal(raw, &m) != nil {
		return
	}
	switch m.Type {
	case "status":
		for _, c := range adapter.News(d.status, m.Status) {
			d.a.applyExt("edge", proto.Message{Type: "cheer", Cheer: &proto.Cheer{Kind: c}})
		}
		d.status = m.Status
	case "stats":
		d.stats = m.Stats
	case "card":
		d.a.edgeCard(m.Card)
		if m.Card.Bounce {
			d.a.applyExt("edge", proto.Message{Type: "cheer", Cheer: &proto.Cheer{Kind: "hop"}})
		}
		return
	case "remove":
		d.a.ui.RemoveCard("x:edge:" + m.ID)
		return
	}
	edgeSays(d.a.ui, d.status, d.stats)
}
