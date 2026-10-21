// Package vibe is the vibe plugin: click Ember in the card while a song
// plays, and the card becomes a stage where Ember dances beside the song's
// words. Ember draws all of it (docs/PLUGINS.md, Stages); the plugin only
// finds the words and tells a song from an ad or a talk, from the media
// events Ember sends while the stage is open. Nothing happens while it's
// closed: no events reach it, no words are looked up.
package vibe

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/Tejas242/ember/internal/config"
	"github.com/Tejas242/ember/pkg/plugin"
)

// LRCLIB is the lyrics service's address.
const LRCLIB = "https://lrclib.net"

// Options are where the plugin looks; the zero values are the real ones.
type Options struct {
	Finder  *Finder       // nil: the cache in CacheDir, LRCLIB, a client that gives up after 10 s
	Lyrics  func() bool   // may the network be asked ([vibe] lyrics); nil reads the config each song
	Timeout time.Duration // a lookup's limit (15 s)
}

type found struct {
	key string
	w   Words
}

// keep is how many songs' words are kept in memory (a song played again is
// there at once; the rest is in the cache on disk).
const keep = 16

// Run serves the vibe plugin until Ember says stop.
func Run(p *plugin.Plugin, o Options) error {
	if o.Finder == nil {
		o.Finder = &Finder{Dir: CacheDir(), Base: LRCLIB, Client: &http.Client{Timeout: 10 * time.Second}}
	}
	if o.Lyrics == nil {
		o.Lyrics = func() bool { c, _ := config.Load(config.Path()); return c.Lyrics }
	}
	if o.Timeout == 0 {
		o.Timeout = 15 * time.Second
	}
	var (
		cur      *plugin.Media
		words    = map[string]*Words{}
		order    []string
		inflight = map[string]bool{}
		results  = make(chan found)
		sent     []byte
	)
	show := func() {
		var w *Words
		if cur != nil {
			w = words[cur.Key]
		}
		d := Page(cur, w)
		b, _ := json.Marshal(d)
		if bytes.Equal(b, sent) {
			return
		}
		if p.Dashboard(d) == nil {
			sent = b
		}
	}
	lookup := func(m *plugin.Media) {
		if m == nil || Classify(m) != Song || words[m.Key] != nil || inflight[m.Key] {
			return
		}
		f := *o.Finder
		if !o.Lyrics() {
			f.Client = nil // the .lrc beside the file and the cache only
		}
		inflight[m.Key] = true
		mm := *m
		go func() {
			ctx, cancel := context.WithTimeout(p.Context(), o.Timeout)
			defer cancel()
			w := f.Find(ctx, &mm)
			select {
			case results <- found{mm.Key, w}:
			case <-p.Context().Done():
			}
		}()
	}
	show() // a page to open, before anything plays
	for {
		select {
		case ev, ok := <-p.Events():
			if !ok {
				return nil
			}
			if ev.Type != "media" {
				continue
			}
			if newSong(cur, ev.Media) {
				lookup(ev.Media)
			}
			cur = ev.Media
			show()
		case f := <-results:
			delete(inflight, f.key)
			w := f.w
			words[f.key] = &w
			if order = append(order, f.key); len(order) > keep {
				delete(words, order[0])
				order = order[1:]
			}
			if cur != nil && cur.Key == f.key {
				show()
			}
		}
	}
}
