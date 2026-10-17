package source

import (
	"bufio"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// The volume and the microphone (PipeWire, through wpctl; `pactl subscribe`
// says when to look).

var volRe = regexp.MustCompile(`Volume:\s*([0-9.]+)(\s*\[MUTED\])?`)

// parseLevel reads `wpctl get-volume`'s answer.
func parseLevel(out []byte) Volume {
	m := volRe.FindSubmatch(out)
	if m == nil {
		return Volume{}
	}
	v, _ := strconv.ParseFloat(string(m[1]), 64)
	return Volume{V: v, Muted: len(m[2]) > 0, OK: true}
}

func readLevel(target string) Volume {
	out, err := exec.Command("wpctl", "get-volume", target).Output()
	if err != nil {
		return Volume{}
	}
	return parseLevel(out)
}

// levelEvent: a pactl subscribe line that means a level may have changed.
func levelEvent(ln string) bool {
	return strings.Contains(ln, "'change' on sink #") || strings.Contains(ln, "'change' on source #") || strings.Contains(ln, "on server")
}

// Levels follows the default output's volume and the default input's mute
// until `pactl subscribe` ends (the sound server restarted). Without pactl
// and wpctl it returns at once.
func Levels(emit Emit) error {
	for _, tool := range []string{"pactl", "wpctl"} {
		if _, err := exec.LookPath(tool); err != nil {
			return errors.New(tool + " missing")
		}
	}
	cmd := exec.Command("pactl", "subscribe")
	cmd.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGTERM}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("pactl subscribe: %w", err)
	}
	sink, src := readLevel("@DEFAULT_AUDIO_SINK@"), readLevel("@DEFAULT_AUDIO_SOURCE@")
	first := sink
	first.First = true
	emit(first)
	events := make(chan struct{}, 1)
	go func() {
		sc := bufio.NewScanner(out)
		for sc.Scan() {
			if levelEvent(sc.Text()) {
				select {
				case events <- struct{}{}:
				default:
				}
			}
		}
		close(events)
	}()
	for range events {
		time.Sleep(25 * time.Millisecond) // a key press sends a burst
		select {
		case <-events:
		default:
		}
		ns, nm := readLevel("@DEFAULT_AUDIO_SINK@"), readLevel("@DEFAULT_AUDIO_SOURCE@")
		if ns.OK && (ns.V != sink.V || ns.Muted != sink.Muted) {
			emit(ns)
		}
		if nm.OK && src.OK && nm.Muted != src.Muted {
			emit(Mic{Muted: nm.Muted})
		}
		sink, src = ns, nm
	}
	cmd.Wait()
	return errors.New("pactl subscribe ended (the sound server restarted?)")
}

// SetVolume sets the default output's volume, 0..1.
func SetVolume(frac float64) {
	exec.Command("wpctl", "set-volume", "@DEFAULT_AUDIO_SINK@", strconv.FormatFloat(max(0, min(1, frac)), 'f', 3, 64)).Run()
}

// NudgeVolume moves it 5% up (dir > 0) or down, never past 100%.
func NudgeVolume(dir int) {
	step := "5%+"
	if dir < 0 {
		step = "5%-"
	}
	exec.Command("wpctl", "set-volume", "-l", "1", "@DEFAULT_AUDIO_SINK@", step).Run()
}

// ToggleMute mutes or unmutes the default output.
func ToggleMute() { exec.Command("wpctl", "set-mute", "@DEFAULT_AUDIO_SINK@", "toggle").Run() }
