package island

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// ---- words to mouth shapes ----
//
// The singing mouth needs, for each line, its syllables and the shapes
// each one takes: the consonants it starts with, its vowel (two, for a
// diphthong: "I" is AI then E), the consonants it ends on. Spelling isn't
// sound, so the words go through a small grapheme-to-phoneme step first:
//
//   - English: a few hundred hand-written rules (digraphs, magic e, open
//     syllables, r-coloured vowels, suffixes, unstressed vowels reduced)
//     and an exceptions list of the words songs use most.
//   - Hindi in Devanagari: almost one to one (each consonant carries an
//     "a" unless a matra or virama says otherwise; the schwa is dropped at
//     the end of a word and between a vowel and a consonant cluster).
//   - Hindi in Latin letters: its own rules ("aa", "ee", "oo", "ai", final
//     e said; "bh", "ph" are lips), picked when a line reads as Hindi.
//   - Other scripts: Arabic letters and Japanese kana have their vowels
//     read off; anything else sings one open syllable a letter.
//
// The phonemes then fall into ten mouth shapes (visemes), after Preston
// Blair's set, that read at 88 px.

// viseme is a mouth shape.
type viseme uint8

const (
	vRest viseme = iota // closed, relaxed
	vMBP                // lips pressed together
	vFV                 // upper teeth on the lower lip
	vTH                 // L and TH: open a little, the tongue up
	vCH                 // SH CH J: lips forward, teeth showing
	vEtc                // C D G K N R S T Y Z: open a little, teeth together
	vAI                 // wide open
	vE                  // wide, half open
	vO                  // round
	vU                  // small round: OO, W
	nVisemes
)

var visemeNames = [nVisemes]string{"rest", "MBP", "FV", "L/TH", "CH", "etc", "AI", "E", "O", "U/W"}

func (v viseme) String() string { return visemeNames[v] }

// phone is a phoneme, ARPAbet-like.
type phone uint8

const (
	phNone phone = iota
	// vowels
	phAA // father, hot
	phAE // cat
	phAH // but
	phAX // about (unstressed)
	phEH // bed
	phIH // bit
	phIY // see
	phEY // day
	phAY // my
	phAW // now
	phOW // go
	phOY // boy
	phAO // saw
	phUH // book
	phUW // blue
	phER // bird
	phEE // Hindi e: (pure)
	phOO // Hindi o: (pure)
	// consonants
	phP
	phB
	phM
	phF
	phV
	phTH
	phDH
	phL
	phW
	phR
	phS
	phZ
	phSH
	phZH
	phCH
	phJH
	phT
	phD
	phN
	phK
	phG
	phNG
	phHH
	phY
	nPhones
)

type phoneInfo struct {
	name   string
	vowel  bool
	v1, v2 viseme  // the shape (v2: a diphthong's second)
	open   float32 // how open the shape goes, 0..1
}

var phoneTab = [nPhones]phoneInfo{
	phAA: {"AA", true, vAI, vAI, 1}, phAE: {"AE", true, vAI, vAI, 0.9}, phAH: {"AH", true, vAI, vAI, 0.75},
	phAX: {"AX", true, vAI, vAI, 0.5}, phEH: {"EH", true, vE, vE, 0.8}, phIH: {"IH", true, vE, vE, 0.6},
	phIY: {"IY", true, vE, vE, 0.5}, phEY: {"EY", true, vE, vE, 0.75}, phAY: {"AY", true, vAI, vE, 1},
	phAW: {"AW", true, vAI, vU, 1}, phOW: {"OW", true, vO, vU, 0.85}, phOY: {"OY", true, vO, vE, 0.85},
	phAO: {"AO", true, vO, vO, 1}, phUH: {"UH", true, vU, vU, 0.7}, phUW: {"UW", true, vU, vU, 0.6},
	phER: {"ER", true, vAI, vAI, 0.55}, phEE: {"EE", true, vE, vE, 0.7}, phOO: {"OO", true, vO, vO, 0.85},
	phP: {"P", false, vMBP, vMBP, 0}, phB: {"B", false, vMBP, vMBP, 0}, phM: {"M", false, vMBP, vMBP, 0},
	phF: {"F", false, vFV, vFV, 0.2}, phV: {"V", false, vFV, vFV, 0.2}, phTH: {"TH", false, vTH, vTH, 0.35},
	phDH: {"DH", false, vTH, vTH, 0.35}, phL: {"L", false, vTH, vTH, 0.4}, phW: {"W", false, vU, vU, 0.3},
	phR: {"R", false, vEtc, vEtc, 0.3}, phS: {"S", false, vEtc, vEtc, 0.2}, phZ: {"Z", false, vEtc, vEtc, 0.2},
	phSH: {"SH", false, vCH, vCH, 0.3}, phZH: {"ZH", false, vCH, vCH, 0.3}, phCH: {"CH", false, vCH, vCH, 0.3},
	phJH: {"JH", false, vCH, vCH, 0.3}, phT: {"T", false, vEtc, vEtc, 0.25}, phD: {"D", false, vEtc, vEtc, 0.25},
	phN: {"N", false, vEtc, vEtc, 0.25}, phK: {"K", false, vEtc, vEtc, 0.3}, phG: {"G", false, vEtc, vEtc, 0.3},
	phNG: {"NG", false, vEtc, vEtc, 0.3}, phHH: {"HH", false, vEtc, vEtc, 0.45}, phY: {"Y", false, vEtc, vEtc, 0.3},
}

var phoneByName = func() map[string]phone {
	m := map[string]phone{}
	for p := phone(1); p < nPhones; p++ {
		m[phoneTab[p].name] = p
	}
	return m
}()

// ---- English ----

// The words songs use most whose spelling the rules get wrong.
const englishExceptions = `a:AX an:AE N the:DH AX to:T UW do:D UW you:Y UW your:Y AO R yours:Y AO R Z youre:Y AO R
i:AY im:AY M ive:AY V id:AY D ill:AY L me:M IY my:M AY we:W IY he:HH IY she:SH IY be:B IY they:DH EY theyre:DH EH R
are:AA R were:W ER was:W AA Z wasnt:W AA Z AX N T is:IH Z isnt:IH Z AX N T its:IH T S of:AH V for:F AO R or:AO R
one:W AH N once:W AH N S two:T UW come:K AH M comes:K AH M Z coming:K AH M IH NG some:S AH M someone:S AH M W AH N
something:S AH M TH IH NG somebody:S AH M B AA D IY sometimes:S AH M T AY M Z done:D AH N none:N AH N love:L AH V
loved:L AH V D lover:L AH V ER lovers:L AH V ER Z loving:L AH V IH NG lovely:L AH V L IY above:AX B AH V glove:G L AH V
move:M UW V prove:P R UW V lose:L UW Z whose:HH UW Z who:HH UW whom:HH UW M whole:HH OW L what:W AH T where:W EH R
there:DH EH R here:HH IH R their:DH EH R heart:HH AA R T hearts:HH AA R T S eye:AY eyes:AY Z said:S EH D says:S EH Z
again:AX G EH N been:B IH N any:EH N IY many:M EH N IY anyone:EH N IY W AH N anything:EH N IY TH IH NG women:W IH M IH N
woman:W UH M AX N does:D AH Z doesnt:D AH Z AX N T gone:G AO N give:G IH V given:G IH V AX N live:L IH V lived:L IH V D
living:L IH V IH NG have:HH AE V having:HH AE V IH NG would:W UH D could:K UH D should:SH UH D wouldnt:W UH D AX N T
couldnt:K UH D AX N T through:TH R UW though:DH OW although:AO L DH OW thought:TH AO T tough:T AH F enough:IH N AH F
laugh:L AE F rough:R AH F friend:F R EH N D friends:F R EH N D Z people:P IY P AX L beautiful:B Y UW T IH F AX L
maybe:M EY B IY only:OW N L IY every:EH V R IY everything:EH V R IY TH IH NG everyone:EH V R IY W AH N everybody:EH V R IY B AA D IY
very:V EH R IY oh:OW ooh:UW ah:AA aah:AA yeah:Y EH AX yea:Y EH ayy:EY ay:AY eh:EH uh:AH hey:HH EY la:L AA na:N AA
girl:G ER L girls:G ER L Z world:W ER L D word:W ER D words:W ER D Z work:W ER K worth:W ER TH worse:W ER S worst:W ER S T
sugar:SH UH G ER sure:SH UH R want:W AA N T wanna:W AA N AX water:W AO T ER watch:W AA CH wash:W AA SH gonna:G AH N AX
gotta:G AA T AX tryna:T R AY N AX cause:K AH Z because:B IH K AH Z island:AY L AX N D heaven:HH EH V AX N ready:R EH D IY
dead:D EH D head:HH EH D bread:B R EH D breath:B R EH TH death:D EH TH great:G R EY T break:B R EY K weather:W EH DH ER
door:D AO R floor:F L AO R blood:B L AH D flood:F L AH D good:G UH D wood:W UH D stood:S T UH D foot:F UH T put:P UH T
push:P UH SH pull:P UH L full:F UH L bush:B UH SH busy:B IH Z IY business:B IH Z N AX S minute:M IH N IH T money:M AH N IY
honey:HH AH N IY son:S AH N front:F R AH N T month:M AH N TH wonder:W AH N D ER won:W AH N mother:M AH DH ER
brother:B R AH DH ER other:AH DH ER another:AX N AH DH ER nothing:N AH TH IH NG color:K AH L ER colour:K AH L ER
cover:K AH V ER oven:AH V AX N among:AX M AH NG tongue:T AH NG young:Y AH NG touch:T AH CH country:K AH N T R IY
double:D AH B AX L trouble:T R AH B AX L cousin:K AH Z AX N buy:B AY guy:G AY bye:B AY fire:F AY ER desire:D IH Z AY ER
quiet:K W AY AX T idea:AY D IY AX real:R IY L really:R IY L IY radio:R EY D IY OW video:V IH D IY OW piano:P IY AE N OW
music:M Y UW Z IH K believe:B IH L IY V either:IY DH ER neither:N IY DH ER height:HH AY T eight:EY T obey:OW B EY
key:K IY christmas:K R IH S M AX S chorus:K AO R AX S echo:EH K OW school:S K UW L listen:L IH S AX N often:AO F AX N
answer:AE N S ER know:N OW knew:N UW knife:N AY F knee:N IY wrong:R AO NG write:R AY T honest:AA N AX S T hour:AW ER
our:AW ER ours:AW ER Z four:F AO R pour:P AO R course:K AO R S alone:AX L OW N together:T AX G EH DH ER forever:F ER EH V ER
never:N EH V ER ever:EH V ER seven:S EH V AX N river:R IH V ER city:S IH T IY pretty:P R IH T IY body:B AA D IY
sorry:S AA R IY story:S T AO R IY party:P AA R T IY baby:B EY B IY lady:L EY D IY today:T AX D EY tonight:T AX N AY T
tomorrow:T AX M AA R OW away:AX W EY around:AX R AW N D about:AX B AW T alive:AX L AY V again:AX G EH N against:AX G EH N S T
along:AX L AO NG already:AO L R EH D IY also:AO L S OW always:AO L W EY Z almost:AO L M OW S T over:OW V ER open:OW P AX N
whatever:W AH T EH V ER forget:F ER G EH T get:G EH T gets:G EH T S getting:G EH T IH NG begin:B IH G IH N gift:G IH F T
oh-oh:OW OW done:D AH N don:D AA N dont:D OW N T wont:W OW N T cant:K AE N T aint:EY N T thats:DH AE T S whats:W AH T S
lets:L EH T S theres:DH EH R Z dancing:D AE N S IH NG dance:D AE N S chance:CH AE N S last:L AE S T fast:F AE S T
past:P AE S T ask:AE S K after:AE F T ER half:HH AE F calm:K AA M palm:P AA M walk:W AO K talk:T AO K
warm:W AO R M war:W AO R small:S M AO L all:AO L call:K AO L fall:F AO L ball:B AO L wall:W AO L tall:T AO L
moment:M OW M AX N T broken:B R OW K AX N frozen:F R OW Z AX N golden:G OW L D AX N open:OW P AX N hello:HH AX L OW
window:W IH N D OW shadow:SH AE D OW yellow:Y EH L OW follow:F AA L OW pillow:P IH L OW tomorrow:T AX M AA R OW
how:HH AW now:N AW wow:W AW cow:K AW allow:AX L AW down:D AW N town:T AW N brown:B R AW N crown:K R AW N
own:OW N grown:G R OW N known:N OW N shown:SH OW N blown:B L OW N flown:F L OW N thrown:TH R OW N
power:P AW ER flower:F L AW ER tower:T AW ER shower:SH AW ER lower:L OW ER bowl:B OW L
here's:HH IH R Z hear:HH IH R near:N IH R fear:F IH R dear:D IH R year:Y IH R clear:K L IH R tear:T IH R ear:IH R
bear:B EH R wear:W EH R swear:S W EH R learn:L ER N earth:ER TH heard:HH ER D early:ER L IY search:S ER CH
care:K EH R share:SH EH R stare:S T EH R dare:D EH R rare:R EH R scared:S K EH R D where:W EH R were:W ER
mind:M AY N D find:F AY N D kind:K AY N D behind:B IH HH AY N D wild:W AY L D child:CH AY L D wind:W IH N D
cold:K OW L D hold:HH OW L D gold:G OW L D told:T OW L D old:OW L D most:M OW S T ghost:G OW S T lost:L AO S T
both:B OW TH sky:S K AY fly:F L AY cry:K R AY try:T R AY why:W AY by:B AY dry:D R AY high:HH AY
go:G OW no:N OW so:S OW oh:OW toe:T OW shoe:SH UW goes:G OW Z
movie:M UW V IY field:F IY L D piece:P IY S die:D AY lie:L AY tie:T AY
true:T R UW blue:B L UW
moment:M OW M AX N T broken:B R OW K AX N frozen:F R OW Z AX N total:T OW T AX L hotel:HH OW T EH L
robot:R OW B AA T hoping:HH OW P IH NG motion:M OW SH AX N ocean:OW SH AX N notice:N OW T IH S focus:F OW K AX S
photo:F OW T OW solo:S OW L OW soda:S OW D AX local:L OW K AX L slowly:S L OW L IY lonely:L OW N L IY
mr:M IH S T ER mrs:M IH S IH Z ok:OW K EY okay:OW K EY
`

var englishLex = func() map[string][]phone {
	m := map[string][]phone{}
	for _, line := range strings.Split(englishExceptions, "\n") {
		f := strings.Fields(line)
		var word string
		var ph []phone
		flush := func() {
			if word != "" && len(ph) > 0 {
				m[word] = ph
			}
		}
		for _, t := range f {
			if w, p, ok := strings.Cut(t, ":"); ok {
				flush()
				word, ph = w, nil
				t = p
			}
			if p, ok := phoneByName[t]; ok {
				ph = append(ph, p)
			}
		}
		flush()
	}
	return m
}()

func isVowelLetter(c byte) bool { return c == 'a' || c == 'e' || c == 'i' || c == 'o' || c == 'u' }

// vowelGroups counts the vowel letter groups in w (y counts after a consonant).
func vowelGroups(w string) int {
	n, in := 0, false
	for i := 0; i < len(w); i++ {
		v := isVowelLetter(w[i]) || (w[i] == 'y' && i > 0 && !isVowelLetter(w[i-1]))
		if v && !in {
			n++
		}
		in = v
	}
	if n > 1 && strings.HasSuffix(w, "e") && !strings.HasSuffix(w, "ee") && !strings.HasSuffix(w, "le") && len(w) > 2 && !isVowelLetter(w[len(w)-2]) {
		n-- // a silent final e
	}
	return n
}

// englishWord appends w's phones (w lower case, letters only) to out.
func englishWord(w string, out []phone) []phone {
	if p, ok := englishLex[w]; ok {
		return append(out, p...)
	}
	n := len(w)
	// suffixes: a known stem and its ending
	type suf struct {
		s   string
		ph  []phone
		min int
	}
	for _, sf := range [...]suf{
		{"ing", []phone{phIH, phNG}, 2}, {"in", []phone{phIH, phN}, 99}, {"ed", nil, 2}, {"es", nil, 2}, {"s", nil, 2},
		{"ly", []phone{phL, phIY}, 3}, {"ness", []phone{phN, phAX, phS}, 3}, {"ful", []phone{phF, phAX, phL}, 3},
		{"less", []phone{phL, phAX, phS}, 3}, {"ment", []phone{phM, phAX, phN, phT}, 3},
	} {
		if !strings.HasSuffix(w, sf.s) || n-len(sf.s) < sf.min {
			continue
		}
		stem := w[:n-len(sf.s)]
		if vowelGroups(stem) == 0 {
			continue
		}
		switch sf.s {
		case "s":
			if strings.HasSuffix(stem, "s") || strings.HasSuffix(stem, "u") || strings.HasSuffix(stem, "i") {
				continue
			}
			out = englishWord(stem, out)
			return append(out, phZ)
		case "es":
			if strings.HasSuffix(stem, "s") || strings.HasSuffix(stem, "x") || strings.HasSuffix(stem, "z") || strings.HasSuffix(stem, "ch") || strings.HasSuffix(stem, "sh") {
				out = englishWord(stem, out)
				return append(out, phIH, phZ)
			}
			if c := stem[len(stem)-1]; c == 'c' || c == 'g' || c == 's' || c == 'z' {
				out = englishWord(stem+"e", out)
				return append(out, phIH, phZ)
			}
			out = englishWord(stem+"e", out)
			return append(out, phZ)
		case "ed":
			last := stem[len(stem)-1]
			base := restoreE(stem)
			out = englishWord(base, out)
			if last == 't' || last == 'd' {
				return append(out, phIH, phD)
			}
			return append(out, phD)
		case "ing":
			out = englishWord(restoreE(stem), out)
			return append(out, sf.ph...)
		default:
			out = englishWord(stem, out)
			return append(out, sf.ph...)
		}
	}
	return englishRules(w, out)
}

// restoreE undoes a suffix's spelling: "hop" (hoped) is "hope", "runn" (running) is "run".
func restoreE(stem string) string {
	k := len(stem)
	if k >= 2 && stem[k-1] == stem[k-2] && !isVowelLetter(stem[k-1]) && stem[k-1] != 'l' && stem[k-1] != 's' {
		return stem[:k-1]
	}
	if _, ok := englishLex[stem+"e"]; ok {
		return stem + "e"
	}
	if k >= 3 && !isVowelLetter(stem[k-1]) && stem[k-1] != 'w' && stem[k-1] != 'x' && stem[k-1] != 'y' &&
		isVowelLetter(stem[k-2]) && !isVowelLetter(stem[k-3]) && vowelGroups(stem) == 1 && stem[k-1] != 'r' {
		return stem + "e"
	}
	return stem
}

// englishRules: the letters, by rule.
func englishRules(w string, out []phone) []phone {
	n := len(w)
	start := len(out)
	at := func(i int) byte {
		if i < 0 || i >= n {
			return 0
		}
		return w[i]
	}
	cons := func(c byte) bool { return c != 0 && !isVowelLetter(c) && c != 'y' }
	has := func(i int, s string) bool { return i >= 0 && strings.HasPrefix(w[i:], s) }
	final := func(i int) bool { return i >= n }
	// magic e: vowel, one consonant, a final e (or e before s/d)
	magic := func(i int) bool {
		c := at(i + 1)
		if !cons(c) || c == 'x' || c == 'w' {
			return false
		}
		if at(i+2) == 'e' && (i+3 == n || (i+4 == n && (at(i+3) == 's' || at(i+3) == 'd'))) {
			return true
		}
		return false
	}
	// open: vowel, one consonant, a vowel (the first syllable of a longer word)
	open := func(i int) bool {
		c := at(i + 1)
		if !cons(c) || c == 'x' || c == 'w' || c == 'r' || c == 'v' {
			return false
		}
		v := at(i + 2)
		return isVowelLetter(v) && !(v == 'e' && i+3 == n) && !(v == 'i' && has(i+2, "ing"))
	}
	firstVowel := true
	for i := 0; i < n; {
		c := w[i]
		vowelHere := isVowelLetter(c) || (c == 'y' && i > 0 && cons(at(i-1)))
		if vowelHere {
			fv := firstVowel
			firstVowel = false
			switch {
			case has(i, "eigh"):
				out, i = append(out, phEY), i+4
			case has(i, "igh"):
				out, i = append(out, phAY), i+3
			case has(i, "augh"):
				out, i = append(out, phAO), i+4
			case has(i, "ough"):
				out, i = append(out, phAO), i+4
			case has(i, "tion"):
				i++
			case has(i, "air"), has(i, "are") && i+3 == n, has(i, "ear") && final(i+3) && false:
				out, i = append(out, phEH, phR), i+3
			case has(i, "ear") && (final(i+3) || isVowelLetter(at(i+3)) || at(i+3) == 's'):
				out, i = append(out, phIH, phR), i+3
			case has(i, "eer"):
				out, i = append(out, phIH, phR), i+3
			case has(i, "ere") && final(i+3):
				out, i = append(out, phIH, phR), i+3
			case has(i, "ire") && (final(i+3) || at(i+3) == 'd' || at(i+3) == 's'):
				out, i = append(out, phAY, phER), i+3
			case has(i, "our"):
				out, i = append(out, phAW, phER), i+3
			case has(i, "ore") && final(i+3):
				out, i = append(out, phAO, phR), i+3
			case has(i, "ee"):
				out, i = append(out, phIY), i+2
			case has(i, "ea"):
				out, i = append(out, phIY), i+2
			case has(i, "ai"), has(i, "ay"):
				out, i = append(out, phEY), i+2
			case has(i, "au"), has(i, "aw"):
				out, i = append(out, phAO), i+2
			case has(i, "ei"):
				out, i = append(out, phEY), i+2
			case has(i, "ey") && final(i+2):
				out, i = append(out, phIY), i+2
			case has(i, "ey"):
				out, i = append(out, phEY), i+2
			case has(i, "eu"), has(i, "ew"):
				out, i = append(out, phUW), i+2
			case has(i, "ie") && final(i+2):
				if vowelGroups(w) == 1 {
					out = append(out, phAY)
				} else {
					out = append(out, phIY)
				}
				i += 2
			case has(i, "ie") && (at(i+2) == 'd' || at(i+2) == 's') && final(i+3):
				if vowelGroups(w) == 1 {
					out = append(out, phAY)
				} else {
					out = append(out, phIY)
				}
				i += 2
			case has(i, "ie"):
				out, i = append(out, phIY), i+2
			case has(i, "oa"):
				out, i = append(out, phOW), i+2
			case has(i, "oe") && final(i+2):
				out, i = append(out, phOW), i+2
			case has(i, "oi"), has(i, "oy"):
				out, i = append(out, phOY), i+2
			case has(i, "ook"):
				out, i = append(out, phUH), i+2
			case has(i, "oo"):
				out, i = append(out, phUW), i+2
			case has(i, "ous") && final(i+3):
				out, i = append(out, phAX, phS), i+3
			case has(i, "ou"):
				out, i = append(out, phAW), i+2
			case has(i, "ow") && (final(i+2) || at(i+2) == 's'):
				if vowelGroups(w) > 1 {
					out = append(out, phOW)
				} else {
					out = append(out, phOW)
				}
				i += 2
			case has(i, "ow"):
				out, i = append(out, phAW), i+2
			case has(i, "ue"), has(i, "ui"):
				if i > 0 && (at(i-1) == 'g' || at(i-1) == 'q') {
					if final(i + 2) {
						i += 2 // tongue, league: silent
						continue
					}
					out, i = append(out, phIH), i+2
					continue
				}
				out, i = append(out, phUW), i+2
			case c == 'a' && (has(i, "all") || has(i, "alk") || has(i, "alt")):
				out, i = append(out, phAO), i+1
			case c == 'a' && at(i+1) == 'r' && !isVowelLetter(at(i+2)) && at(i+2) != 'r' && at(i+2) != 'y':
				out, i = append(out, phAA, phR), i+2
			case c == 'a' && i > 0 && at(i-1) == 'w' && at(i+1) == 'r':
				out, i = append(out, phAO, phR), i+2
			case c == 'a' && i > 0 && at(i-1) == 'w' && cons(at(i+1)) && !magic(i):
				out, i = append(out, phAA), i+1
			case c == 'o' && at(i+1) == 'r' && !isVowelLetter(at(i+2)) && at(i+2) != 'r':
				if i > 0 && at(i-1) == 'w' {
					out = append(out, phER)
				} else {
					out = append(out, phAO, phR)
				}
				i += 2
			case (c == 'e' || c == 'i' || c == 'u' || c == 'y') && at(i+1) == 'r' && !isVowelLetter(at(i+2)) && at(i+2) != 'r' && at(i+2) != 'y':
				out, i = append(out, phER), i+2
			case c == 'i' && (has(i, "ind") || has(i, "ild")) && final(i+3):
				out, i = append(out, phAY), i+1
			case c == 'o' && (has(i, "old") || has(i, "olt") || has(i, "oll")):
				out, i = append(out, phOW), i+1
			case c == 'e' && final(i+1) && i > 0:
				if vowelGroups(w) <= 1 && start == len(out) || !anyVowel(out[start:]) {
					out = append(out, phIY) // he, me, be
				}
				i++
			case c == 'e' && i == n-2 && (at(i+1) == 's' || at(i+1) == 'd') && anyVowel(out[start:]):
				i++ // a silent e before s or d
			case magic(i) || (fv && open(i) && (c == 'a' || c == 'u')):
				out, i = append(out, longVowel(c)), i+1
			case c == 'y' && final(i+1):
				if !anyVowel(out[start:]) {
					out = append(out, phAY)
				} else {
					out = append(out, phIY)
				}
				i++
			case (c == 'o' || c == 'a' || c == 'i' || c == 'u') && final(i+1):
				switch c {
				case 'o':
					out = append(out, phOW)
				case 'a':
					out = append(out, phAH)
				case 'i':
					out = append(out, phIY)
				case 'u':
					out = append(out, phUW)
				}
				i++
			default:
				out, i = append(out, shortVowel(c, fv)), i+1
			}
			continue
		}
		// consonants
		switch {
		case c == 'y' && i == 0:
			out, i = append(out, phY), i+1
		case c == 'y':
			out, i = append(out, phY), i+1
		case has(i, "tion"), has(i, "sion") && i > 0 && !isVowelLetter(at(i-1)):
			out, i = append(out, phSH, phAX, phN), i+4
		case has(i, "sion"):
			out, i = append(out, phZH, phAX, phN), i+4
		case has(i, "ture"):
			out, i = append(out, phCH, phER), i+4
		case has(i, "sure"):
			out, i = append(out, phSH, phER), i+4
		case (has(i, "cial") || has(i, "tial")) && i > 0:
			out, i = append(out, phSH, phAX, phL), i+4
		case c == 'l' && at(i+1) == 'e' && final(i+2) && i > 0 && cons(at(i-1)):
			out, i = append(out, phAX, phL), i+2
		case has(i, "tch"):
			out, i = append(out, phCH), i+3
		case has(i, "sch"):
			out, i = append(out, phS, phK), i+3
		case has(i, "chr"):
			out, i = append(out, phK), i+2
		case has(i, "ch"):
			out, i = append(out, phCH), i+2
		case has(i, "sh"):
			out, i = append(out, phSH), i+2
		case has(i, "th"):
			out, i = append(out, phTH), i+2
		case has(i, "ph"):
			out, i = append(out, phF), i+2
		case has(i, "wh"):
			out, i = append(out, phW), i+2
		case has(i, "ck"):
			out, i = append(out, phK), i+2
		case has(i, "ng"):
			out, i = append(out, phNG), i+2
		case has(i, "gh"):
			if i == 0 {
				out = append(out, phG)
			}
			i += 2
		case i == 0 && (has(i, "kn") || has(i, "gn")):
			out, i = append(out, phN), i+2
		case i == 0 && has(i, "wr"):
			out, i = append(out, phR), i+2
		case i == 0 && has(i, "ps"):
			out, i = append(out, phS), i+2
		case has(i, "mb") && final(i+2):
			out, i = append(out, phM), i+2
		case has(i, "dg"):
			out, i = append(out, phJH), i+2
		case has(i, "qu"):
			out, i = append(out, phK, phW), i+2
		case c == 'x':
			if i == 0 {
				out = append(out, phZ)
			} else {
				out = append(out, phK, phS)
			}
			i++
		case c == 'c':
			if nx := at(i + 1); nx == 'e' || nx == 'i' || nx == 'y' {
				out = append(out, phS)
			} else if nx == 'c' {
				if nn := at(i + 2); nn == 'e' || nn == 'i' {
					out = append(out, phK, phS)
					i += 2
					continue
				}
				out = append(out, phK)
				i++
			} else {
				out = append(out, phK)
			}
			i++
		case c == 'g':
			if nx := at(i + 1); (nx == 'e' || nx == 'i' || nx == 'y') && (i > 0 || !hardG(w)) {
				out = append(out, phJH)
			} else if nx == 'g' {
				out = append(out, phG)
				i++
			} else {
				out = append(out, phG)
			}
			i++
		case c == 'l' && i > 0 && (has(i, "lk") || has(i, "lm")) && at(i-1) == 'a':
			i++ // walk, calm
		case c == 'j':
			out, i = append(out, phJH), i+1
		case c == 'q':
			out, i = append(out, phK), i+1
		case c == 's' && i > 0 && isVowelLetter(at(i-1)) && at(i+1) == 'e' && final(i+2):
			out, i = append(out, phZ), i+1
		case i+1 < n && w[i+1] == c: // a double consonant is one
			out = append(out, letterPhone(c))
			i += 2
		default:
			if p := letterPhone(c); p != phNone {
				out = append(out, p)
			}
			i++
		}
	}
	reduce(w, out[start:])
	return out
}

// hardG: a word starting ge, gi with a hard g (get, give, girl).
func hardG(w string) bool {
	for _, p := range [...]string{"get", "giv", "gift", "girl", "gear", "geese", "gig", "gild", "gimme", "gil", "gir"} {
		if strings.HasPrefix(w, p) {
			return true
		}
	}
	return false
}

func anyVowel(ps []phone) bool {
	for _, p := range ps {
		if phoneTab[p].vowel {
			return true
		}
	}
	return false
}

func longVowel(c byte) phone {
	switch c {
	case 'a':
		return phEY
	case 'e':
		return phIY
	case 'i', 'y':
		return phAY
	case 'o':
		return phOW
	}
	return phUW
}

func shortVowel(c byte, first bool) phone {
	switch c {
	case 'a':
		return phAE
	case 'e':
		return phEH
	case 'i', 'y':
		return phIH
	case 'o':
		return phAA
	}
	return phAH
}

func letterPhone(c byte) phone {
	switch c {
	case 'b':
		return phB
	case 'd':
		return phD
	case 'f':
		return phF
	case 'h':
		return phHH
	case 'k':
		return phK
	case 'l':
		return phL
	case 'm':
		return phM
	case 'n':
		return phN
	case 'p':
		return phP
	case 'r':
		return phR
	case 's':
		return phS
	case 't':
		return phT
	case 'v':
		return phV
	case 'w':
		return phW
	case 'z':
		return phZ
	case 'g':
		return phG
	case 'c', 'q':
		return phK
	case 'j':
		return phJH
	case 'y':
		return phY
	}
	return phNone
}

// reduce: the stressed syllable keeps its vowel, short vowels elsewhere go
// to a schwa. Stress falls on the first syllable, or the second after a
// prefix.
func reduce(w string, ps []phone) {
	nv := 0
	for _, p := range ps {
		if phoneTab[p].vowel {
			nv++
		}
	}
	if nv < 2 {
		return
	}
	stress := 0
	for _, pre := range [...]string{"a", "be", "de", "re", "un", "con", "com", "ex", "for", "per", "pre", "dis", "mis", "sur", "en", "em", "in", "im"} {
		if strings.HasPrefix(w, pre) && len(w) > len(pre)+2 {
			stress = 1
			break
		}
	}
	k := 0
	for i, p := range ps {
		if !phoneTab[p].vowel {
			continue
		}
		if k != stress {
			switch p {
			case phAE, phAH, phAA:
				ps[i] = phAX
			case phEH:
				ps[i] = phIH
			}
		}
		k++
	}
}

// ---- Hindi in Latin letters ----

var romanHindiLex = func() map[string]bool {
	m := map[string]bool{}
	for _, w := range strings.Fields(`hai hain mein main mai tu tum hum ham tera teri tere mera meri mere kya ki ka ke ko se na nahi nahin
		dil pyaar pyar ishq jaan yaar ho hoon hu jo yeh ye woh wo aur bhi toh kahin kabhi phir sab saath raat din
		zindagi mujhe tujhe humko tumko apna apni apne koi kuch bas tujh mujh dekh chal aaja jaana jana diya
		gaya gayi kar karo kare mohabbat sapna sapne aankhen ankhein dard khuda rabba rab sajna sanam mahiya ve tainu
		menu nu hun kyun kyon jab tab ab hoga hogi tha thi rahe raha rahi ek baat baatein`) {
		m[w] = true
	}
	return m
}()

// romanHindi says whether a line in Latin letters reads as Hindi.
func romanHindi(line string) bool {
	hi, en, n := 0, 0, 0
	for _, w := range strings.FieldsFunc(strings.ToLower(line), func(r rune) bool { return !unicode.IsLetter(r) && r != '\'' }) {
		n++
		if romanHindiLex[w] {
			hi++
		} else if _, ok := englishLex[strings.ReplaceAll(w, "'", "")]; ok {
			en++
		}
	}
	return hi >= 2 && hi > en || (hi >= 1 && n <= 3 && en == 0)
}

// romanHindiWord: Hindi as it's written in Latin letters.
func romanHindiWord(w string, out []phone) []phone {
	n := len(w)
	has := func(i int, s string) bool { return strings.HasPrefix(w[i:], s) }
	for i := 0; i < n; {
		c := w[i]
		switch {
		case has(i, "aa"):
			out, i = append(out, phAA), i+2
		case has(i, "ai"), has(i, "ae"):
			out, i = append(out, phEH), i+2
		case has(i, "au"), has(i, "aw"):
			out, i = append(out, phAO), i+2
		case has(i, "ee"), has(i, "ii"):
			out, i = append(out, phIY), i+2
		case has(i, "oo"), has(i, "uu"):
			out, i = append(out, phUW), i+2
		case c == 'a':
			if i == n-1 {
				out = append(out, phAA) // mera, kya: a final a is long
			} else {
				out = append(out, phAH)
			}
			i++
		case c == 'e':
			out, i = append(out, phEE), i+1
		case c == 'i':
			if i == n-1 {
				out = append(out, phIY)
			} else {
				out = append(out, phIH)
			}
			i++
		case c == 'o':
			out, i = append(out, phOO), i+1
		case c == 'u':
			out, i = append(out, phUH), i+1
		case c == 'y' && i == n-1 && i > 0:
			out, i = append(out, phIY), i+1
		case has(i, "chh"):
			out, i = append(out, phCH), i+3
		case has(i, "bh"):
			out, i = append(out, phB), i+2
		case has(i, "ph"):
			out, i = append(out, phP), i+2 // फ: an aspirated p, the lips close
		case has(i, "sh"):
			out, i = append(out, phSH), i+2
		case has(i, "ch"):
			out, i = append(out, phCH), i+2
		case has(i, "jh"):
			out, i = append(out, phJH), i+2
		case has(i, "th"):
			out, i = append(out, phT), i+2
		case has(i, "dh"):
			out, i = append(out, phD), i+2
		case has(i, "kh"):
			out, i = append(out, phK), i+2
		case has(i, "gh"):
			out, i = append(out, phG), i+2
		case has(i, "ng"):
			out, i = append(out, phNG), i+2
		case c == 'v':
			out, i = append(out, phV), i+1
		case c == 'x':
			out, i = append(out, phK, phS), i+1
		case c == 'q':
			out, i = append(out, phK), i+1
		case i+1 < n && w[i+1] == c:
			out, i = append(out, letterPhone(c)), i+2
		default:
			if p := letterPhone(c); p != phNone {
				out = append(out, p)
			}
			i++
		}
	}
	return out
}

// ---- Devanagari ----

// devaConsonant is the phone of a Devanagari consonant (U+0915..U+0939, U+0958..U+095F).
func devaConsonant(r rune) phone {
	switch r {
	case 'क', 'ख', 'ग', 'घ', '\u0958', '\u0959', '\u095A':
		return phK
	case 'ङ':
		return phNG
	case 'च', 'छ':
		return phCH
	case 'ज', 'झ', '\u095B':
		return phJH
	case 'ञ', 'ण', 'न', 'ऩ':
		return phN
	case 'ट', 'ठ', 'त', 'थ':
		return phT
	case 'ड', 'ढ', 'द', 'ध', '\u095C', '\u095D':
		return phD
	case 'प', 'फ':
		return phP
	case '\u095E':
		return phF
	case 'ब', 'भ':
		return phB
	case 'म':
		return phM
	case 'य', '\u095F':
		return phY
	case 'र', 'ऱ':
		return phR
	case 'ल', 'ळ':
		return phL
	case 'व':
		return phV
	case 'श', 'ष':
		return phSH
	case 'स':
		return phS
	case 'ह':
		return phHH
	}
	return phNone
}

func devaVowel(r rune) (phone, bool) {
	switch r {
	case 'अ':
		return phAH, true
	case 'आ', 'ा':
		return phAA, true
	case 'इ', 'ि':
		return phIH, true
	case 'ई', 'ी':
		return phIY, true
	case 'उ', 'ु':
		return phUH, true
	case 'ऊ', 'ू':
		return phUW, true
	case 'ए', 'े':
		return phEE, true
	case 'ऐ', 'ै':
		return phEH, true
	case 'ओ', 'ो':
		return phOO, true
	case 'औ', 'ौ':
		return phAO, true
	case 'ऋ', 'ृ':
		return phIH, true // ri
	case 'ॅ', 'ऍ':
		return phAE, true
	case 'ॉ', 'ऑ':
		return phAO, true
	}
	return phNone, false
}

// devaWord: a word of Devanagari. Each consonant carries an inherent "a"
// unless a matra or a virama follows; that schwa is dropped at the word's
// end and, from the right, where a vowel comes before and a vowel after
// the next consonant (kam-ra, not ka-ma-ra).
func devaWord(w string, out []phone) []phone {
	start := len(out)
	var schwa [64]bool // out index (from start) of an inherent a
	rs := []rune(w)
	for i := 0; i < len(rs); i++ {
		r := rs[i]
		if r == '़' { // a nukta: already folded in, or ignored
			continue
		}
		if p, ok := devaVowel(r); ok {
			out = append(out, p)
			continue
		}
		if r == 'ं' || r == 'ँ' { // a nasal: m before the lips, else n (chandrabindu only colours the vowel)
			if r == 'ं' {
				nx := phNone
				if i+1 < len(rs) {
					nx = devaConsonant(rs[i+1])
				}
				if nx == phP || nx == phB || nx == phM {
					out = append(out, phM)
				} else {
					out = append(out, phN)
				}
			}
			continue
		}
		if r == 'ः' {
			out = append(out, phHH)
			continue
		}
		c := devaConsonant(r)
		if i+1 < len(rs) && rs[i+1] == '़' {
			switch c {
			case phP:
				c = phF
			case phJH:
				c = phZ
			}
		}
		if c == phNone {
			continue
		}
		out = append(out, c)
		j := i + 1
		for j < len(rs) && rs[j] == '़' {
			j++
		}
		if j < len(rs) && rs[j] == '्' { // virama: no vowel
			i = j
			continue
		}
		if j < len(rs) {
			if _, ok := devaVowel(rs[j]); ok && (rs[j] < 'अ' || rs[j] > 'औ') { // a matra follows
				continue
			}
		}
		if k := len(out) - start; k < len(schwa) {
			schwa[k] = true
		}
		out = append(out, phAH)
	}
	// schwa deletion
	seg := out[start:]
	keep := make([]bool, len(seg))
	for i := range keep {
		keep[i] = true
	}
	isV := func(i int) bool { return i >= 0 && i < len(seg) && keep[i] && phoneTab[seg[i]].vowel }
	nv := 0
	for _, p := range seg {
		if phoneTab[p].vowel {
			nv++
		}
	}
	if k := len(seg) - 1; nv > 1 && k > 0 && k < len(schwa) && schwa[k] {
		keep[k] = false
	}
	for i := len(seg) - 2; i > 1; i-- {
		if i >= len(schwa) || !schwa[i] || !keep[i] {
			continue
		}
		// V C [a] C V
		if isV(i-2) && !phoneTab[seg[i-1]].vowel && i+2 < len(seg) && !phoneTab[seg[i+1]].vowel && isV(i+2) {
			keep[i] = false
		}
	}
	k := start
	for i, p := range seg {
		if keep[i] {
			out[k] = p
			k++
		}
	}
	return out[:k]
}

// ---- other scripts ----

// arabicWord reads vowels off an Arabic word: alef, waw and yeh are long
// vowels (or w, y before a vowel), every other letter a consonant carrying
// a short a unless a long vowel follows.
func arabicWord(w string, out []phone) []phone {
	rs := []rune(w)
	for i, r := range rs {
		var c phone
		switch r {
		case 'ا', 'آ', 'أ', 'إ', 'ى':
			out = append(out, phAA)
			continue
		case 'و':
			if i == 0 {
				c = phW
			} else {
				out = append(out, phUW)
				continue
			}
		case 'ي':
			if i == 0 {
				c = phY
			} else {
				out = append(out, phIY)
				continue
			}
		case 'ب', 'م':
			c = phM
		case 'ف':
			c = phF
		case 'ث', 'ذ', 'ظ', 'ل':
			c = phTH
		case 'ش', 'ج':
			c = phSH
		case 'ة':
			out = append(out, phAH)
			continue
		default:
			if r < 0x621 || r > 0x64a {
				continue
			}
			c = phT
		}
		out = append(out, c)
		next := rune(0)
		if i+1 < len(rs) {
			next = rs[i+1]
		}
		if i+1 < len(rs) && next != 'ا' && next != 'و' && next != 'ي' && next != 'ى' && i > 0 {
			out = append(out, phAH)
		} else if i == 0 && next != 'ا' && next != 'و' && next != 'ي' {
			out = append(out, phAH)
		}
	}
	return out
}

// kanaRomaji is the hiragana block (U+3041..U+3096) in romaji; katakana
// maps onto it. The small tsu ("q") doubles the next consonant: no sound.
const kanaRomaji = "a a i i u u e e o o ka ga ki gi ku gu ke ge ko go sa za shi ji su zu se ze so zo " +
	"ta da chi ji q tsu zu te de to do na ni nu ne no ha ba pa hi bi pi fu bu pu he be pe ho bo po " +
	"ma mi mu me mo ya ya yu yu yo yo ra ri ru re ro wa wa wi we wo n vu ka ke"

var kanaTable = func() [][]phone {
	var t [][]phone
	for _, k := range strings.Fields(kanaRomaji) {
		if k == "q" {
			t = append(t, nil)
			continue
		}
		t = append(t, romanHindiWord(k, nil))
	}
	return t
}()

// kanaPhones appends a kana's phones.
func kanaPhones(r rune, out []phone) []phone {
	if r >= 0x30a1 && r <= 0x30f6 { // katakana to hiragana
		r -= 0x60
	}
	if r == 0x30fc && len(out) > 0 { // ー: the vowel goes on
		return out
	}
	if k := int(r - 0x3041); k >= 0 && k < len(kanaTable) {
		return append(out, kanaTable[k]...)
	}
	return out
}

// ---- syllables ----

// syl is one sung syllable: the shapes it starts with, its vowel (and a
// diphthong's second shape), the shapes it ends on, and how long it's sung
// relative to the line's others.
type syl struct {
	on     [3]viseme
	nOn    uint8
	v1, v2 viseme
	o1, o2 float32 // how open each vowel shape goes
	coda   [3]viseme
	nCoda  uint8
	w      float32 // relative length
	r0, r1 int     // the runes of the line it's written with (for labels)
	vowel  phone
	word   int // the word of the line it's in
}

// lineSyllables splits a line into its sung syllables, appended to out.
func lineSyllables(text string, out []syl) []syl {
	hindi := romanHindi(text)
	var buf [96]phone
	rs := []rune(text)
	words := 0
	for i := 0; i < len(rs); {
		if !unicode.IsLetter(rs[i]) && !unicode.Is(unicode.Mn, rs[i]) && !unicode.Is(unicode.Mc, rs[i]) {
			i++
			continue
		}
		j := i
		for j < len(rs) && (unicode.IsLetter(rs[j]) || unicode.Is(unicode.Mn, rs[j]) || unicode.Is(unicode.Mc, rs[j]) || (rs[j] == '\'' && j+1 < len(rs) && unicode.IsLetter(rs[j+1])) || rs[j] == '’') {
			if j > i && scriptOf(rs[j]) != scriptOf(rs[i]) && scriptOf(rs[j]) != 0 {
				break
			}
			j++
		}
		word := string(rs[i:j])
		if j < len(rs) && (rs[j] == '\'' || rs[j] == '’') && strings.HasSuffix(strings.ToLower(word), "in") {
			word += "'" // holdin'
		}
		ps := wordPhones(word, hindi, buf[:0])
		k := len(out)
		out = syllabify(ps, i, j, out)
		for ; k < len(out); k++ {
			out[k].word = words
		}
		if len(ps) > 0 {
			words++
		}
		i = j
	}
	if len(out) > 0 { // a line's last syllable is held
		out[len(out)-1].w *= 1.8
	}
	return out
}

// scriptOf: 1 Latin, 2 Devanagari, 3 Arabic, 4 kana, 5 Han, 0 marks and the rest.
func scriptOf(r rune) int {
	switch {
	case r < 0x250 && unicode.IsLetter(r):
		return 1
	case r >= 0x900 && r <= 0x97f:
		return 2
	case r >= 0x600 && r <= 0x6ff:
		return 3
	case r >= 0x3040 && r <= 0x30ff:
		return 4
	case unicode.Is(unicode.Han, r):
		return 5
	}
	if unicode.IsLetter(r) {
		return 6
	}
	return 0
}

// wordPhones: a word's phones in whichever script it's written.
func wordPhones(word string, hindi bool, out []phone) []phone {
	r, _ := utf8.DecodeRuneInString(word)
	switch scriptOf(r) {
	case 1:
		lw := strings.ToLower(word)
		if strings.HasSuffix(lw, "in'") || strings.HasSuffix(lw, "in’") { // holdin'
			lw = strings.TrimRight(lw, "'’") + "g"
		}
		lw = strings.Map(func(r rune) rune {
			if r >= 'a' && r <= 'z' {
				return r
			}
			switch r {
			case 'é', 'è', 'ê':
				return 'e'
			case 'á', 'à', 'â':
				return 'a'
			case 'í', 'ï':
				return 'i'
			case 'ó', 'ö':
				return 'o'
			case 'ú', 'ü':
				return 'u'
			}
			return -1
		}, lw)
		if lw == "" {
			return out
		}
		if hindi && (romanHindiLex[lw] || englishLex[lw] == nil) {
			return romanHindiWord(lw, out)
		}
		return englishWord(lw, out)
	case 2:
		return devaWord(word, out)
	case 3:
		return arabicWord(word, out)
	case 4:
		for _, r := range word {
			out = kanaPhones(r, out)
		}
		return out
	}
	for range []rune(word) { // anything else: one open syllable a letter
		out = append(out, phT, phAH)
	}
	return out
}

// syllabify groups a word's phones into syllables: each vowel a nucleus,
// one consonant between vowels starts the next syllable, more split (the
// first ends the syllable before).
func syllabify(ps []phone, r0, r1 int, out []syl) []syl {
	var vs [32]int
	nv := 0
	for i, p := range ps {
		if phoneTab[p].vowel && nv < len(vs) {
			vs[nv] = i
			nv++
		}
	}
	if nv == 0 {
		if len(ps) == 0 {
			return out
		}
		// no vowel ("hmm", "mm"): one syllable on its consonants
		s := syl{v1: phoneTab[ps[0]].v1, v2: phoneTab[ps[0]].v1, o1: 0.3, o2: 0.3, w: 1, r0: r0, r1: r1}
		return append(out, s)
	}
	span := r1 - r0
	for k := 0; k < nv; k++ {
		vi := vs[k]
		lo := 0
		if k > 0 {
			gap := vi - vs[k-1] - 1
			lo = vs[k-1] + 1
			if gap >= 2 {
				lo++
			}
		}
		hi := len(ps)
		if k+1 < nv {
			gap := vs[k+1] - vi - 1
			hi = vi + 1
			if gap >= 2 {
				hi++
			}
		}
		p := phoneTab[ps[vi]]
		s := syl{v1: p.v1, v2: p.v2, o1: p.open, o2: p.open, w: 1, vowel: ps[vi]}
		if p.v2 != p.v1 {
			s.o2 = 0.6 * p.open
			if p.v2 == vU {
				s.o2 = 0.45
			}
		}
		for i := lo; i < vi && s.nOn < 3; i++ {
			s.on[s.nOn] = phoneTab[ps[i]].v1
			s.nOn++
		}
		for i := vi + 1; i < hi && s.nCoda < 3; i++ {
			s.coda[s.nCoda] = phoneTab[ps[i]].v1
			s.nCoda++
		}
		switch ps[vi] {
		case phAX, phIH, phUH:
			s.w = 0.75
		case phAA, phIY, phUW, phEY, phAY, phAW, phOW, phOY, phAO, phEE, phOO:
			s.w = 1.15
		}
		s.w += 0.12 * float32(s.nCoda)
		s.r0 = r0 + span*k/nv
		s.r1 = r0 + span*(k+1)/nv
		out = append(out, s)
	}
	return out
}
