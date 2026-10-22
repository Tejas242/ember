#!/usr/bin/env python3
"""The lip-sync test song: original lines sung by espeak-ng over a made-up beat.

    python3 song.py OUTDIR

writes OUTDIR/song.wav (48 kHz stereo, the voice in the centre, the band
panned), OUTDIR/song.lrc (its lines' times) and OUTDIR/truth.json (every
phoneme's time in the mix, from espeak-ng's own phoneme events). Needs
numpy and libespeak-ng. Every word here was written for this test.
"""
import ctypes, json, math, os, sys
import numpy as np

SR = 48000
BPM = 96
BEAT = 60 / BPM
EIGHTH = BEAT / 2

# (voice, text (|what espeak reads, word for word), pitch per word (espeak
# 0..99), the last word held for beats). The romanised Hindi line is sung
# from its Devanagari, as such a song is.
LINES = [
    ("en", "maybe the moon will burn a little brighter", [50, 55, 60, 58, 55, 52, 50, 48], 1.5),
    ("en", "paper boats are sailing past my window", [52, 56, 54, 58, 55, 50, 48], 1.5),
    ("en", "hold me while the city sings so slow", [60, 58, 55, 52, 54, 56, 52, 48], 3.0),
    ("hi", "मेरा मन बस गाता रहे", [52, 56, 54, 50, 48], 2.0),
    ("hi", "phir se gao, dil ki baat|फिर से गाओ दिल की बात", [55, 52, 58, 54, 50, 48], 2.0),
    ("en", "we were only humming at the stars", [50, 52, 55, 58, 54, 50, 52], 2.5),
]

lib = ctypes.CDLL("libespeak-ng.so.1")


class EV(ctypes.Structure):
    _fields_ = [("type", ctypes.c_int), ("uid", ctypes.c_uint), ("text_position", ctypes.c_int), ("length", ctypes.c_int),
                ("audio_position", ctypes.c_int), ("sample", ctypes.c_int), ("user_data", ctypes.c_void_p), ("id", ctypes.c_char * 8)]


CB = ctypes.CFUNCTYPE(ctypes.c_int, ctypes.POINTER(ctypes.c_short), ctypes.c_int, ctypes.POINTER(EV))
ESR = lib.espeak_Initialize(2, 0, None, 1)
_buf, _ev = [], []


def _cb(wav, n, ev):
    if n > 0:
        _buf.extend(wav[:n])
    i = 0
    while ev[i].type != 0:
        if ev[i].type == 7:
            _ev.append((ev[i].sample, ev[i].id.decode("latin1")))
        i += 1
    return 0


_cbf = CB(_cb)
lib.espeak_SetSynthCallback(_cbf)


def say(voice, text, pitch, rate=125):
    """One word: its samples at SR and its phonemes [(sample, name)]."""
    _buf.clear(); _ev.clear()
    lib.espeak_SetVoiceByName(voice.encode())
    lib.espeak_SetParameter(1, rate, 0)   # rate, wpm
    lib.espeak_SetParameter(3, pitch, 0)  # pitch
    lib.espeak_SetParameter(4, 20, 0)     # range: a sung note is flat-ish
    t = text.encode("utf8")
    lib.espeak_Synth(t, len(t) + 1, 0, 0, 0, 1, None, None)
    lib.espeak_Synchronize()
    x = np.array(_buf, dtype=np.float64) / 32768
    ph = [(s, n) for s, n in _ev if n not in ("_", "_:", "_!", "||", "|") and not n.startswith("(")]
    n2 = int(round(len(x) * SR / ESR))  # band-limited resample to SR
    X = np.fft.rfft(x)
    Y = np.zeros(n2 // 2 + 1, dtype=complex)
    m = min(len(X), len(Y))
    Y[:m] = X[:m]
    y = np.fft.irfft(Y, n2) * (n2 / len(x))
    return y, [(int(round(s * SR / ESR)), n) for s, n in ph]


def is_vowel(name):
    n = name.lstrip("',;%=")
    return bool(n) and n[0] in "aeiouAEIOUV0@3"


def hold(y, ph, extra):
    """Sustains the word's last vowel by extra seconds: whole pitch periods
    of its middle repeated, crossfaded (a held note)."""
    vs = [i for i, (_, n) in enumerate(ph) if is_vowel(n)]
    if not vs or extra <= 0:
        return y, ph
    i = vs[-1]
    v0 = ph[i][0]
    v1 = ph[i + 1][0] if i + 1 < len(ph) else len(y) - int(0.05 * SR)
    mid = (v0 + v1) // 2
    seg = y[mid - 1200: mid + 1200]
    ac = np.correlate(seg, seg, "full")[len(seg) - 1:]
    lo, hi = SR // 400, SR // 70
    per = lo + int(np.argmax(ac[lo:hi]))
    cyc = y[mid: mid + 4 * per].copy()
    reps = int(extra * SR / len(cyc)) + 1
    fade = np.linspace(0, 1, per)
    loop = []
    for r in range(reps):
        c = cyc.copy()
        c[:per] = c[:per] * fade + y[mid - per: mid] * (1 - fade) if r == 0 else c[:per]
        loop.append(c)
    loop = np.concatenate(loop)[: int(extra * SR)]
    vib = 1 + 0.08 * np.sin(2 * np.pi * 5.5 * np.arange(len(loop)) / SR) * np.minimum(1, np.arange(len(loop)) / SR / 0.4)
    loop = loop * vib
    out = np.concatenate([y[:mid], loop, y[mid:]])
    n = len(loop)
    return out, [(s if s < mid else s + n, nm) for s, nm in ph]


def env(n, a, d):
    t = np.arange(n) / SR
    return np.minimum(1, t / max(a, 1e-4)) * np.exp(-t / d)


def pan(x, p):  # p -1 left .. +1 right, constant power
    th = (p + 1) * math.pi / 4
    return np.stack([x * math.cos(th), x * math.sin(th)], 1)


def main(out):
    os.makedirs(out, exist_ok=True)
    rng = np.random.default_rng(7)
    lead = 4 * BEAT * 2  # two bars of beat first
    t = lead
    voice_parts, truth, lrc = [], [], []
    for li, (voice, text, pitches, held) in enumerate(LINES):
        text, _, sung = text.partition("|")
        words = (sung or text).replace(",", "").split()
        line = {"line": li, "text": text, "phonemes": []}
        first = None
        for wi, w in enumerate(words):
            y, ph = say(voice, w, pitches[wi % len(pitches)])
            if wi == len(words) - 1:
                y, ph = hold(y, ph, held * BEAT * 0.8)
            vs = [s for s, n in ph if is_vowel(n)]
            v0 = vs[0] if vs else 0
            nsyl = max(1, len(vs))
            start = t - v0 / SR  # the first vowel on the beat
            if first is None:
                first = start + (ph[0][0] / SR if ph else 0)
            voice_parts.append((start, y))
            for s, n in ph:
                line["phonemes"].append({"t": round(start + s / SR, 4), "ph": n, "vowel": is_vowel(n), "word": wi})
            dur = len(y) / SR - v0 / SR
            slots = max(nsyl, math.ceil(dur / EIGHTH - 0.35))
            t += slots * EIGHTH
        line["start"] = round(first, 3)
        line["end"] = round(t, 3)
        truth.append(line)
        lrc.append((first, text))
        t += 2 * BEAT
        t = math.ceil(t / BEAT) * BEAT
    total = t + 2 * BEAT
    n = int(total * SR)
    mix = np.zeros((n, 2))
    vo = np.zeros(n)
    for start, y in voice_parts:
        i = int(start * SR)
        vo[i: i + len(y)] += y[: max(0, n - i)]
    vo *= 0.5 / (np.max(np.abs(vo)) + 1e-9)
    mix += pan(vo, 0)

    nbeats = int(total / BEAT)
    chords = [(220.0, 277.18, 329.63), (196.0, 246.94, 293.66), (174.61, 220.0, 261.63), (196.0, 246.94, 329.63)]
    roots = [55.0, 49.0, 43.65, 49.0]
    for b in range(nbeats):
        bt = b * BEAT
        i = int(bt * SR)
        bar = (b // 4) % 4
        if b % 4 in (0, 2) or (b % 4 == 3 and b % 8 == 7):  # kick, centre
            m = int(0.35 * SR)
            tt = np.arange(m) / SR
            f = 45 + 80 * np.exp(-tt * 30)
            k = np.sin(2 * np.pi * np.cumsum(f) / SR) * np.exp(-tt * 9) * 0.55
            mix[i: i + m] += pan(k, 0)[: n - i]
        if b % 4 in (1, 3):  # snare, a little right
            m = int(0.2 * SR)
            tt = np.arange(m) / SR
            s = (rng.standard_normal(m) * 0.6 * np.exp(-tt * 22) + np.sin(2 * np.pi * 185 * tt) * 0.4 * np.exp(-tt * 30)) * 0.22
            mix[i: i + m] += pan(s, 0.2)[: n - i]
        for h in range(2):  # hats, right
            j = i + int(h * EIGHTH * SR)
            m = int(0.05 * SR)
            hn = rng.standard_normal(m)
            hn = np.diff(np.concatenate([[0], hn])) * np.exp(-np.arange(m) / SR * 80) * 0.08
            mix[j: j + m] += pan(hn, 0.65)[: max(0, n - j)]
        # bass, centre: root, an eighth on and off
        m = int(BEAT * SR)
        tt = np.arange(m) / SR
        bs = (np.sin(2 * np.pi * roots[bar] * tt) + 0.25 * np.sin(4 * np.pi * roots[bar] * tt)) * env(m, 0.01, 0.4) * 0.22
        mix[i: i + m] += pan(bs, 0)[: n - i]
        # pluck arpeggio, left of centre, right in the voice's band
        for e in range(2):
            j = i + int(e * EIGHTH * SR)
            f = chords[bar][(2 * b + e) % 3] * 2
            m = int(0.3 * SR)
            tt = np.arange(m) / SR
            pl = (np.sin(2 * np.pi * f * tt) + 0.5 * np.sin(4 * np.pi * f * tt) + 0.25 * np.sin(6 * np.pi * f * tt)) * env(m, 0.003, 0.12) * 0.09
            mix[j: j + m] += pan(pl, -0.55)[: max(0, n - j)]
        # pad: the chord, wide (each side detuned its own way)
        if b % 4 == 0:
            m = int(4 * BEAT * SR)
            tt = np.arange(m) / SR
            a = np.minimum(1, tt / 0.3) * np.minimum(1, (4 * BEAT - tt) / 0.3)
            L = sum(np.sin(2 * np.pi * f * 1.003 * tt + k) for k, f in enumerate(chords[bar])) * a * 0.035
            R = sum(np.sin(2 * np.pi * f * 0.997 * tt + 2 * k + 1) for k, f in enumerate(chords[bar])) * a * 0.035
            mix[i: i + m, 0] += L[: n - i]
            mix[i: i + m, 1] += R[: n - i]
    mix *= 0.9 / np.max(np.abs(mix))
    pcm = (mix * 32767).astype("<i2")
    import wave
    with wave.open(os.path.join(out, "song.wav"), "wb") as w:
        w.setnchannels(2); w.setsampwidth(2); w.setframerate(SR)
        w.writeframes(pcm.tobytes())
    with open(os.path.join(out, "song.lrc"), "w") as f:
        for at, text in lrc:
            f.write("[%02d:%05.2f]%s\n" % (int(at // 60), at % 60, text))
        f.write("[%02d:%05.2f]\n" % (int(total // 60), total % 60 - 1))
    with open(os.path.join(out, "truth.json"), "w") as f:
        json.dump({"rate": SR, "length": total, "lines": truth}, f, ensure_ascii=False, indent=1)
    print("%.1f s, %d lines" % (total, len(truth)))


if __name__ == "__main__":
    main(sys.argv[1])
