package island

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Tejas242/ember/internal/source"
)

// `ember doctor` hears how each source is doing, and why one isn't.
func TestStatusHasSources(t *testing.T) {
	clock := time.Now()
	a := newTestApp(&clock)
	a.sup = source.NewSupervisor()
	a.sup.Min, a.sup.Logf = time.Hour, func(string, ...any) {}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go a.sup.Run(ctx, "volume", func(source.Emit) error { return errors.New("pactl missing") }, func(source.Event) {})
	for i := 0; i < 500; i++ {
		if st := a.status(); len(st.Sources) == 1 && !st.Sources[0].OK {
			if s := st.Sources[0]; s.Name != "volume" || s.Detail != "pactl missing" {
				t.Fatalf("%+v", s)
			}
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("status never said why volume stopped")
}
