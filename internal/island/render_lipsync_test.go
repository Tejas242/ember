//go:build render

package island

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/Tejas242/ember/internal/source"
)

// EMBER_LIPSYNC=dir EMBER_LIPSYNC_OUT=out go test -tags render -count=1 -run TestLipSync ./internal/island
//
// The test song (testdata/lipsync/song.py writes dir/song.wav, song.lrc and
// truth.json) played on the vibe stage at real speed: its PCM goes through
// the real analysis (source.Audio.FeedPCM), 20 ms at a time as parec would
// hand it over (30 ms late), and Ember sings it. Out come the stage and a
// close-up of the face as videos with the song as their soundtrack, a strip
// of one line's syllables with the shape each got, and how far each
// syllable's shape landed from when it's sung (espeak-ng's own phoneme
// times), against the old way (a vowel looked up at a linear fill).

type lsPhoneme struct {
	T     float64 `json:"t"`
	Ph    string  `json:"ph"`
	Vowel bool    `json:"vowel"`
	Word  int     `json:"word"`
}

type lsLine struct {
	Line     int         `json:"line"`
	Text     string      `json:"text"`
	Start    float64     `json:"start"`
	End      float64     `json:"end"`
	Phonemes []lsPhoneme `json:"phonemes"`
}

type lsTruth struct {
	Rate   int      `json:"rate"`
	Length float64  `json:"length"`
	Lines  []lsLine `json:"lines"`
}

// songPCM reads a 16-bit stereo WAV and brings it down to Ember's rate
// (a windowed-sinc low-pass under 3.6 kHz, then every n-th sample).
func songPCM(t *testing.T, path string) ([]int16, int) {
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	rate := int(binary.LittleEndian.Uint32(b[24:]))
	i := 12
	for i+8 < len(b) && string(b[i:i+4]) != "data" {
		i += 8 + int(binary.LittleEndian.Uint32(b[i+4:]))
	}
	d := b[i+8:]
	n := len(d) / 4
	dec := rate / source.AudioRate
	const taps = 127
	var h [taps]float64
	fc := 3600.0 / float64(rate)
	for k := range h {
		x := float64(k - taps/2)
		s := 2 * fc
		if x != 0 {
			s = math.Sin(2*math.Pi*fc*x) / (math.Pi * x)
		}
		h[k] = s * (0.42 - 0.5*math.Cos(2*math.Pi*float64(k)/(taps-1)) + 0.08*math.Cos(4*math.Pi*float64(k)/(taps-1)))
	}
	out := make([]int16, 0, 2*n/dec+2)
	for o := 0; o*dec < n; o++ {
		for c := 0; c < 2; c++ {
			s := 0.0
			for k := 0; k < taps; k++ {
				j := o*dec + k - taps/2
				if j >= 0 && j < n {
					s += h[k] * float64(int16(binary.LittleEndian.Uint16(d[4*j+2*c:])))
				}
			}
			out = append(out, int16(math.Max(-32768, math.Min(32767, s))))
		}
	}
	return out, rate
}

// lsFeed hears a hop of the song: the real analysis.
var lsFeed = func(a *source.Audio, hop []int16, at time.Time) { a.FeedPCM(hop, at) }

// a video made by ffmpeg from frames piped in, with the song under it.
type lsVideo struct {
	cmd  *exec.Cmd
	in   io.WriteCloser
	w, h int
}

func newVideo(t *testing.T, out string, w, h int, wav string) *lsVideo {
	cmd := exec.Command("ffmpeg", "-y", "-loglevel", "error", "-f", "rawvideo", "-pix_fmt", "rgba", "-s", fmt.Sprintf("%dx%d", w, h), "-r", "24", "-i", "-",
		"-i", wav, "-map", "0:v", "-map", "1:a", "-c:v", "libx264", "-preset", "medium", "-crf", "18", "-pix_fmt", "yuv420p", "-c:a", "aac", "-b:a", "160k", "-shortest", out)
	in, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	return &lsVideo{cmd, in, w, h}
}

func (v *lsVideo) frame(img *image.RGBA) {
	b := img.Rect
	for y := 0; y < v.h; y++ {
		row := img.Pix[(y)*img.Stride : y*img.Stride+v.w*4]
		if y >= b.Dy() {
			row = make([]byte, v.w*4)
		}
		v.in.Write(row)
	}
}

func (v *lsVideo) close() { v.in.Close(); v.cmd.Wait() }

// lsTick is what the singing mouth did on one frame.
type lsTick struct {
	t      float64 // the song's time (the words' clock)
	key    string
	cur    int
	dom    viseme // the shape pulling hardest
	domSyl int    // whose it is (-1 none)
	closed bool   // the lips closed (pressed or resting)
	h      float32
	off    float64     // the words' offset applied (s, nudged or learnt): the words' clock is t + off
	img    *image.RGBA // the face, for the strip (only near line 0)
}

func TestLipSync(t *testing.T) {
	learnOffsets = true // the harness measures the learner too
	defer func() { learnOffsets = false }()
	dir, out := os.Getenv("EMBER_LIPSYNC"), os.Getenv("EMBER_LIPSYNC_OUT")
	if dir == "" || out == "" {
		t.Skip("set EMBER_LIPSYNC and EMBER_LIPSYNC_OUT")
	}
	t.Setenv("HOME", t.TempDir())
	os.MkdirAll(out, 0o755)
	var truth lsTruth
	if b, err := os.ReadFile(filepath.Join(dir, "truth.json")); err != nil || json.Unmarshal(b, &truth) != nil {
		t.Fatal("no truth.json", err)
	}
	wav := filepath.Join(dir, "song.wav")
	pcm, _ := songPCM(t, wav)
	lrc, err := os.ReadFile(filepath.Join(dir, "song.lrc"))
	if err != nil {
		t.Fatal(err)
	}
	shift := os.Getenv("EMBER_LIPSYNC_SHIFT") // move the .lrc's times (ms): words files are rarely exact
	if shift != "" {
		var ms int
		fmt.Sscan(shift, &ms)
		var b strings.Builder
		for _, ln := range strings.Split(strings.TrimSpace(string(lrc)), "\n") {
			var m int
			var s float64
			if _, err := fmt.Sscanf(ln, "[%d:%f]", &m, &s); err == nil {
				at := float64(m)*60 + s + float64(ms)/1000
				fmt.Fprintf(&b, "[%02d:%05.2f]%s\n", int(at/60), math.Mod(at, 60), ln[strings.Index(ln, "]")+1:])
			}
		}
		lrc = []byte(b.String())
	}

	g := newScene()
	g.work("k", "kinvault", "fix the race in the store", 3)
	g.music(true)
	tr := g.a.ui.track
	tr.Title, tr.Artist, tr.Album = "Paper Boats", "Test Tone", ""
	tr.Length = time.Duration(truth.Length * float64(time.Second))
	sd := t.TempDir()
	os.WriteFile(filepath.Join(sd, "song.lrc"), lrc, 0o644)
	tr.URL = "file://" + filepath.Join(sd, "song.flac")
	tr.Position, tr.PosAt = 0, g.clock
	g.a.ui.audio = &source.Audio{Now: func() time.Time { return g.clock }}
	if os.Getenv("EMBER_LIPSYNC_NOLEARN") != "" { // a nudge of 0 set by hand: it wins, nothing is learnt
		g.a.ui.nudges().Ms[mediaKey(tr)] = 0
	}
	b := startVibe(t, g)
	g.a.ui.openDash("vibe")
	b.settle(false)
	tr = g.a.ui.track
	u := g.a.ui
	var last pose
	debugPose = &last
	defer func() { debugPose = nil }()

	var stage, face *lsVideo
	var stageR, faceR image.Rectangle
	const faceK = 5
	hop := 0
	var ticks []lsTick
	nofilm := os.Getenv("EMBER_LIPSYNC_NOFILM") != "" // the numbers only
	aligned := map[string][]float64{}
	start := g.clock
	for i := 0; ; i++ {
		g.clock = start.Add(time.Duration(i+1) * time.Second / 60)
		pos := tr.Pos(g.clock).Seconds()
		for hop*2*source.AudioHop+2*source.AudioHop <= len(pcm) && float64(hop+1)*0.02 <= pos-0.03 {
			heard := g.clock.Add(-time.Duration((pos - 0.03 - float64(hop+1)*0.02) * float64(time.Second)))
			lsFeed(u.audio, pcm[hop*2*source.AudioHop:(hop+1)*2*source.AudioHop], heard)
			hop++
		}
		u.renderDamage()
		s := &u.face.sing
		off := u.wordsNudge(mediaKey(tr)).Seconds()
		if s.key != "" {
			aligned[s.key] = append(aligned[s.key][:0], s.onsets...)
			for k := range aligned[s.key] {
				aligned[s.key][k] -= off // on the song's clock
			}
		}
		tk := lsTick{t: pos, key: s.key, cur: s.cur, domSyl: -1, off: off}
		if last.mouth == mouthSing {
			tk.dom, tk.domSyl = s.dominant(pos + off)
			m := last.sing
			tk.h = m.h * last.mouthK
			tk.closed = tk.h < 0.07
		} else {
			tk.closed = true
		}
		if nofilm {
			ticks = append(ticks, tk)
			if pos >= truth.Length-0.05 {
				break
			}
			continue
		}
		if stage == nil {
			stageR = vibeCrop(u, 0)
			stage = newVideo(t, filepath.Join(out, "stage.mp4"), stageR.Dx()*2, stageR.Dy()*2, wav)
			fx, fy := int(u.spot.x), stageBase-27
			faceR = image.Rect(fx-30, fy-20, fx+30, fy+30)
			face = newVideo(t, filepath.Join(out, "face.mp4"), faceR.Dx()*faceK, faceR.Dy()*faceK, wav)
		}
		need := len(truth.Lines) > 0 && pos > truth.Lines[0].Start-0.3 && pos < truth.Lines[0].End+0.3
		if i%5 == 0 || i%5 == 2 || need {
			full := onDesktop(u, image.Rect(0, 0, u.W, u.H))
			if need {
				tk.img = upscale(full.SubImage(faceR), faceK)
			}
			if i%5 == 0 || i%5 == 2 {
				stage.frame(upscale(full.SubImage(stageR), 2))
				face.frame(upscale(full.SubImage(faceR), faceK))
			}
		}
		ticks = append(ticks, tk)
		if pos >= truth.Length-0.05 {
			break
		}
	}
	if !nofilm {
		stage.close()
		face.close()
	}
	rep := lsMeasure(t, truth, ticks, aligned, string(lrc))
	if o := &u.offset; o.lines > 0 { // what each line offered for the song's offset
		var ob strings.Builder
		fmt.Fprintf(&ob, "\nthe offset: lines %d, onsets' lag %+.0f ms, %d lines agree, believed %v\n", o.lines, o.est*1000, o.support, o.conf)
		for i := 0; i < min(o.lines, offLines); i++ {
			fmt.Fprintf(&ob, "line %2d proposes", i)
			for _, p := range o.props[i][:o.np[i]] {
				fmt.Fprintf(&ob, " %+5.0f/%.2f", p.r*1000, p.w)
			}
			ob.WriteString("\n")
		}
		rep += ob.String()
	}
	os.WriteFile(filepath.Join(out, "sync.txt"), []byte(rep), 0o644)
	t.Log("\n" + rep)
	if !nofilm {
		lsStrip(t, filepath.Join(out, "strip.png"), truth.Lines[0], ticks)
	}
}

// dominant is the timeline's strongest shape at t, and its syllable.
func (s *singer) dominant(t float64) (viseme, int) {
	best, v, k := -1.0, vRest, -1
	for _, g := range s.segs[:s.nseg] {
		d := visemeDom[g.v]
		w := d.a
		switch {
		case t < g.a:
			w *= math.Exp(-(g.a - t) / d.before)
		case t > g.b:
			w *= math.Exp(-(t - g.b) / d.after)
		}
		if w > best {
			best, v, k = w, g.v, int(g.syl)
		}
	}
	return v, k
}

// lsSyl is a sung syllable: when its vowel starts (espeak-ng's time), its
// consonants before, and the syllable it is in Ember's reading.
type lsSyl struct {
	line, k    int
	text       string
	vowelAt    float64
	consAt     float64
	lips       bool // it starts on m, b or p
	v1         viseme
	oldAt      float64 // the old way: when the vowel looked up at the linear fill turns to it
	planAt     float64 // the plan alone (even spacing, no audio)
	onsetAt    float64 // where the alignment put its onset
	alignedAt  float64 // when the alignment showed it (the onset, once heard)
	shapeAt    float64 // when its vowel's shape took over the mouth
	closedOK   bool    // the lips closed for its m, b, p
	closedLate bool    // ... or by 100 ms into its vowel
}

// lsMatch pairs the truth's vowels with Ember's syllables, word by word,
// where they count the same.
func lsMatch(truth lsTruth) (out []lsSyl, total int) {
	for li, ln := range truth.Lines {
		text, _, _ := strings.Cut(ln.Text, "|")
		syls := lineSyllables(text, nil)
		words := map[int][]lsPhoneme{}
		for _, p := range ln.Phonemes {
			words[p.Word] = append(words[p.Word], p)
		}
		for w := 0; w < len(words); w++ {
			var vs []int
			for i, p := range words[w] {
				if p.Vowel {
					vs = append(vs, i)
				}
			}
			total += len(vs)
			var mine []int
			for k, y := range syls {
				if y.word == w {
					mine = append(mine, k)
				}
			}
			if len(mine) != len(vs) {
				continue
			}
			ps := words[w]
			for n, vi := range vs {
				lo := 0
				if n > 0 {
					lo = vs[n-1] + 1
				}
				ca := ps[vi].T
				lips := false
				for j := vi - 1; j >= lo; j-- {
					ca = ps[j].T
					ph := strings.TrimRight(ps[j].Ph, "#")
					if ph == "m" || ph == "b" || ph == "p" {
						lips = true
					}
				}
				y := syls[mine[n]]
				out = append(out, lsSyl{line: li, k: mine[n], text: string([]rune(text)[y.r0:y.r1]), vowelAt: ps[vi].T, consAt: ca, lips: lips && y.nOn > 0, v1: y.v1})
			}
		}
	}
	return out, total
}

func lsMeasure(t *testing.T, truth lsTruth, ticks []lsTick, aligned map[string][]float64, lrc string) string {
	syls, total := lsMatch(truth)
	lrcAt := map[string]float64{}
	var lrcTimes []float64
	for _, ln := range strings.Split(strings.TrimSpace(lrc), "\n") {
		var m int
		var s float64
		if _, err := fmt.Sscanf(ln, "[%d:%f]", &m, &s); err == nil {
			lrcAt[ln[strings.Index(ln, "]")+1:]] = float64(m)*60 + s
			lrcTimes = append(lrcTimes, float64(m)*60+s)
		}
	}
	sort.Float64s(lrcTimes)
	for i := range syls {
		y := &syls[i]
		text, _, _ := strings.Cut(truth.Lines[y.line].Text, "|")
		at := lrcAt[text]
		end := at + 4
		for _, x := range lrcTimes {
			if x > at {
				end = x
				break
			}
		}
		// the old way (natural): fill = (pos+150 ms - at) / min(end-at, 68 ms a letter + 600 ms); the
		// vowel looked up at fill·letters is the syllable's from 4 letters before its vowel letter
		n := utf8.RuneCountInString(text)
		dur := math.Min(end-at, float64(n)*0.068+0.6)
		ss := lineSyllables(text, nil)
		rs := []rune(strings.ToLower(text))
		vl := ss[y.k].r0
		for vl < len(rs) && !strings.ContainsRune("aeiouy", rs[vl]) && rs[vl] < 0x900 {
			vl++
		}
		y.oldAt = at - 0.15 + dur*math.Max(0, float64(vl-4))/float64(n)
		// the plan alone
		var s singer
		s.plan(singLine{text: text, at: time.Duration(at * float64(time.Second)), next: time.Duration(end * float64(time.Second)), hasNext: true})
		acc := s.start
		for k := 0; k < y.k; k++ {
			acc += float64(s.syls[k].w) * s.rate
		}
		y.planAt = acc
		y.alignedAt, y.shapeAt, y.onsetAt = math.NaN(), math.NaN(), math.NaN()
		if a := aligned[text]; y.k < len(a) {
			y.onsetAt = a[y.k]
		}
		for _, tk := range ticks {
			if tk.key != text {
				continue
			}
			if math.IsNaN(y.alignedAt) && tk.cur >= y.k {
				y.alignedAt = tk.t // first shown
			}
			if math.IsNaN(y.shapeAt) && tk.domSyl == y.k && tk.dom == y.v1 && tk.t > y.consAt-0.25 {
				y.shapeAt = tk.t
			}
			if y.lips && tk.t >= y.consAt-0.04 && tk.t <= y.vowelAt+0.04 && tk.closed {
				y.closedOK = true
			}
			if y.lips && tk.t >= y.consAt-0.04 && tk.t <= y.vowelAt+0.1 && tk.closed {
				y.closedLate = true
			}
		}
	}
	var b strings.Builder
	stat := func(name string, f func(y lsSyl) float64) {
		var e []float64
		for _, y := range syls {
			if v := f(y); !math.IsNaN(v) {
				e = append(e, v)
			}
		}
		if len(e) == 0 {
			return
		}
		abs := make([]float64, len(e))
		mean := 0.0
		for i, v := range e {
			abs[i] = math.Abs(v)
			mean += v
		}
		mean /= float64(len(e))
		sort.Float64s(abs)
		within := 0
		for _, v := range abs {
			if v <= 0.1 {
				within++
			}
		}
		fmt.Fprintf(&b, "%-38s n %3d  mean %+5.0f ms  median|e| %4.0f ms  p90|e| %4.0f ms  within 100 ms %3.0f%%\n", name, len(e), mean*1000,
			abs[len(abs)/2]*1000, abs[len(abs)*9/10]*1000, 100*float64(within)/float64(len(e)))
	}
	fmt.Fprintf(&b, "syllables sung %d, matched to Ember's reading %d (%.0f%%)\n", total, len(syls), 100*float64(len(syls))/float64(total))
	stat("old: vowel at the linear fill", func(y lsSyl) float64 { return y.oldAt - y.vowelAt })
	stat("plan alone: even spacing", func(y lsSyl) float64 { return y.planAt - y.vowelAt })
	stat("aligned onset (where it's placed)", func(y lsSyl) float64 { return y.onsetAt - y.vowelAt })
	stat("aligned onset, when shown", func(y lsSyl) float64 { return y.alignedAt - y.vowelAt })
	stat("viseme onset (vowel shape takes over)", func(y lsSyl) float64 { return y.shapeAt - y.vowelAt })
	lips, closed, late := 0, 0, 0
	for _, y := range syls {
		if y.lips {
			lips++
			if y.closedOK {
				closed++
			}
			if y.closedLate {
				late++
			}
		}
	}
	fmt.Fprintf(&b, "m/b/p onsets with the lips closed between the consonant and the vowel: %d of %d (by 100 ms into the vowel: %d)\n", closed, lips, late)
	fmt.Fprintf(&b, "\nper line: the offset applied as it starts (ms, + the words earlier), aligned onsets within 100 ms, viseme onsets within 100 ms\n")
	for li, ln := range truth.Lines {
		off := math.NaN()
		for _, tk := range ticks {
			if tk.t >= ln.Start {
				off = tk.off
				break
			}
		}
		var n, ia, iv int
		for _, y := range syls {
			if y.line != li {
				continue
			}
			n++
			if math.Abs(y.onsetAt-y.vowelAt) <= 0.1 {
				ia++
			}
			if math.Abs(y.shapeAt-y.vowelAt) <= 0.1 {
				iv++
			}
		}
		if n > 0 {
			fmt.Fprintf(&b, "line %2d  offset %+5.0f  aligned %3.0f%%  viseme %3.0f%%  (%d syllables)\n", li, off*1000, 100*float64(ia)/float64(n), 100*float64(iv)/float64(n), n)
		}
	}
	{
		var n, ia, iv int
		for _, y := range syls {
			if y.line >= 5 {
				n++
				if math.Abs(y.onsetAt-y.vowelAt) <= 0.1 {
					ia++
				}
				if math.Abs(y.shapeAt-y.vowelAt) <= 0.1 {
					iv++
				}
			}
		}
		if n > 0 {
			fmt.Fprintf(&b, "lines 5 on: aligned %3.0f%%  viseme %3.0f%%  (%d syllables)\n", 100*float64(ia)/float64(n), 100*float64(iv)/float64(n), n)
		}
	}
	fmt.Fprintf(&b, "\nper syllable (s from the vowel's true onset): text, viseme, old, plan, aligned, shape\n")
	for _, y := range syls {
		fmt.Fprintf(&b, "%d %-8s %-5s %+6.3f %+6.3f %+6.3f %+6.3f %v\n", y.line, y.text, y.v1, y.oldAt-y.vowelAt, y.planAt-y.vowelAt, y.alignedAt-y.vowelAt, y.shapeAt-y.vowelAt, map[bool]string{true: "lips closed", false: ""}[y.closedOK])
	}
	return b.String()
}

// lsStrip: one line, a face for each syllable at the middle of its vowel
// (and at its consonant when it starts on the lips), labelled with the
// syllable and the shape.
func lsStrip(t *testing.T, path string, ln lsLine, ticks []lsTick) {
	text, _, _ := strings.Cut(ln.Text, "|")
	var tr lsTruth
	tr.Lines = []lsLine{ln}
	syls, _ := lsMatch(tr)
	type shot struct {
		img        *image.RGBA
		top, under string
	}
	var shots []shot
	at := func(tt float64) *lsTick {
		var best *lsTick
		for i := range ticks {
			if ticks[i].img != nil && ticks[i].key == text && (best == nil || math.Abs(ticks[i].t-tt) < math.Abs(best.t-tt)) {
				best = &ticks[i]
			}
		}
		return best
	}
	for i, y := range syls {
		next := ln.End
		if i+1 < len(syls) {
			next = syls[i+1].consAt
		}
		if y.lips {
			if tk := at((y.consAt + y.vowelAt) / 2); tk != nil {
				shots = append(shots, shot{tk.img, y.text + " (onset)", tk.dom.String()})
			}
		}
		if tk := at(y.vowelAt + math.Min(0.09, (next-y.vowelAt)/2)); tk != nil {
			shots = append(shots, shot{tk.img, y.text, tk.dom.String()})
		}
	}
	if len(shots) == 0 {
		return
	}
	cw, ch := shots[0].img.Rect.Dx(), shots[0].img.Rect.Dy()
	cols := min(len(shots), 8)
	rows := (len(shots) + cols - 1) / cols
	const lab = 22
	out := newCanvas(cols*cw, rows*(ch+2*lab)+30)
	draw.Draw(out.img, out.img.Rect, image.NewUniform(color.NRGBA{0x12, 0x13, 0x12, 255}), image.Point{}, draw.Src)
	out.text(fBody(), "“"+text+"”: each syllable mid-vowel (and its m, b, p), the shape it gets", 10, 20, color.NRGBA{0xee, 0xee, 0xe8, 255})
	for i, s := range shots {
		x, y := (i%cols)*cw, 30+(i/cols)*(ch+2*lab)
		draw.Draw(out.img, image.Rect(x, y+lab, x+cw, y+lab+ch), s.img, s.img.Rect.Min, draw.Src)
		out.text(fBody(), s.top, float32(x+8), float32(y+16), color.NRGBA{0xff, 0xd8, 0x9a, 255})
		out.text(fCaption(), s.under, float32(x+8), float32(y+lab+ch+16), color.NRGBA{0xcc, 0xcc, 0xc4, 255})
	}
	savePNG(t, path, out.img)
}

// TestLipSyncShapes: the ten mouth shapes at 16, 24 and 88 px, a row each.
func TestLipSyncShapes(t *testing.T) {
	out := os.Getenv("EMBER_LIPSYNC_OUT")
	if out == "" {
		t.Skip("set EMBER_LIPSYNC_OUT")
	}
	g := newScene()
	u := g.a.ui
	f := fireOf(moodIdle)
	cell := func(ps pose, sz float32, w, h, k int) *image.RGBA {
		c := newCanvas(w, h)
		draw.Draw(c.img, c.img.Rect, image.NewUniform(color.NRGBA{0x1a, 0x1c, 0x1a, 255}), image.Point{}, draw.Src)
		c.drawFlame(u.pal, float32(w/2), float32(h)-0.22*sz-2, sz, ps, f, 1)
		return upscale(c.img, k)
	}
	const col = 186
	c := newCanvas(len(visemeNames)*col+20, 560)
	draw.Draw(c.img, c.img.Rect, image.NewUniform(color.NRGBA{0x10, 0x12, 0x10, 255}), image.Point{}, draw.Src)
	for v := viseme(0); v < nVisemes; v++ {
		ps := restPose()
		ps.mouth, ps.mouthK, ps.eyeH = mouthSing, 1, 3
		ps.sing = visemeShapes[v]
		ps.sing.h *= 0.9 // as sung, a strong voice
		x := 10 + int(v)*col
		big := cell(ps, 88, 150, 170, 1)
		draw.Draw(c.img, image.Rect(x, 30, x+150, 200), big, image.Point{}, draw.Src)
		zoom := upscale(big.SubImage(image.Rect(45, 100, 105, 160)), 3)
		draw.Draw(c.img, image.Rect(x, 206, x+180, 386), zoom, image.Point{}, draw.Src)
		p24 := cell(ps, 24, 30, 34, 3)
		draw.Draw(c.img, image.Rect(x, 394, x+90, 496), p24, image.Point{}, draw.Src)
		p16 := cell(ps, 16, 22, 24, 3)
		draw.Draw(c.img, image.Rect(x+94, 394, x+160, 466), p16, image.Point{}, draw.Src)
		c.text(fBody(), v.String(), float32(x+6), 20, color.NRGBA{0xff, 0xd8, 0x9a, 255})
	}
	c.text(fCaption(), "88 px (the stage), its face at 3x, then 24 px and 16 px at 3x", 10, float32(c.img.Rect.Dy()-14), color.NRGBA{0xcc, 0xcc, 0xc4, 255})
	savePNG(t, filepath.Join(out, "shapes.png"), c.img)
}
