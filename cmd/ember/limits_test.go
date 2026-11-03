package main

import (
	"encoding/json"
	"testing"
	"time"
)

func TestLimitsJSON(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	in := `{"model":{},"rate_limits":{"five_hour":{"used_percentage":84.2,"resets_at":1800003600},"seven_day":{"used_percentage":31,"resets_at":1800400000}}}`
	b, ok := limitsJSON([]byte(in), now)
	if !ok {
		t.Fatal("not ok")
	}
	var got map[string]any
	json.Unmarshal(b, &got)
	if got["h5"] != 84.2 || got["h5_at"] != 1800003600.0 || got["d7"] != 31.0 || got["t"] != 1_800_000_000.0 {
		t.Fatalf("%s", b)
	}
	b, _ = limitsJSON([]byte(`{"rate_limits":{"five_hour":{"used_percentage":10,"resets_at":5}}}`), now)
	json.Unmarshal(b, &got)
	if got["d7"] != nil || got["d7_at"] != nil {
		t.Fatalf("a missing 7 day window is null: %s", b)
	}
	for _, bad := range []string{``, `{`, `{}`, `{"rate_limits":{}}`} {
		if _, ok := limitsJSON([]byte(bad), now); ok {
			t.Errorf("%q wrote a file", bad)
		}
	}
}
