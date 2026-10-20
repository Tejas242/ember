package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Tejas242/ember/internal/ipc"
	proto "github.com/Tejas242/ember/pkg/plugin"
)

// `ember note` and `ember status`: the plugin protocol's moment and status,
// for scripts, over the island's socket.

func usageErr(cmd, f string, a ...any) int {
	fmt.Fprintf(os.Stderr, "ember %s: %s (see ember help %s)\n", cmd, fmt.Sprintf(f, a...), cmd)
	return 2
}

func runNote(args []string, f map[string]string) int {
	m := proto.Moment{Text: args[0], Sub: f["sub"], Icon: f["icon"], Mood: f["mood"], URL: f["open"]}
	if v, ok := f["ttl"]; ok {
		s, err := strconv.ParseFloat(strings.TrimSuffix(v, "s"), 64)
		if err != nil || s <= 0 {
			return usageErr("note", "--ttl is seconds, 1 to 10")
		}
		m.TTLs = s
	}
	if u := m.URL; u != "" && !strings.Contains(u, "://") {
		return usageErr("note", "--open needs a URL (https://…, file://…)")
	}
	b, _ := proto.Line("moment", m)
	return cmdOrFail(ipc.Note(b))
}

func runStatus(args []string, f map[string]string) int {
	id := args[0]
	if !proto.NameRe.MatchString(id) {
		return usageErr("status", "the id is 1–32 of a-z, 0-9 and -, not %q", id)
	}
	if f["clear"] == "true" {
		if len(args) > 1 || len(f) > 1 {
			return usageErr("status", "--clear takes only the id")
		}
		return cmdOrFail(ipc.ClearStatus(id))
	}
	if len(args) < 2 {
		return usageErr("status", "needs the line's text, or --clear")
	}
	s := proto.Status{ID: id, Text: args[1], Sub: f["sub"], Icon: f["icon"], Mood: f["mood"], Urgency: f["urgency"], TTLs: 600}
	if v, ok := f["progress"]; ok {
		p, err := strconv.ParseFloat(strings.TrimSuffix(v, "%"), 64)
		if strings.HasSuffix(v, "%") {
			p /= 100
		}
		if err != nil || p < 0 || p > 1 {
			return usageErr("status", "--progress is 0..1 (or 0%%..100%%)")
		}
		s.Progress = &p
	}
	if v, ok := f["since"]; ok {
		ago := time.Duration(0)
		if v != "now" {
			d, err := time.ParseDuration(v)
			if err != nil || d < 0 {
				return usageErr("status", "--since is now, or how long ago: 5m")
			}
			ago = d
		}
		s.SinceMs = time.Now().Add(-ago).UnixMilli()
	}
	if v, ok := f["ttl"]; ok {
		d, err := time.ParseDuration(v)
		if err != nil || d <= 0 || d > 24*time.Hour {
			return usageErr("status", "--ttl is a duration up to 24h: 10m, 2h")
		}
		s.TTLs = d.Seconds()
	}
	b, err := proto.Line("status", s)
	if err != nil {
		return 1
	}
	return cmdOrFail(ipc.SetStatus(id, b))
}
