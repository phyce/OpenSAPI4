package text

import "slices"

// InjectKind is what a tag given to the engine's Inject changes.
type InjectKind int

const (
	InjectGain InjectKind = iota + 1
	InjectReset
	InjectPitch
	InjectSpeed
	InjectVolume
	InjectCharacter
	InjectVoice
)

// The strings that set the gain, 1 to 3, each when it is the whole string.
var gains = []string{`\gain=1\`, `\gain=2\`, `\gain=3\`}

// The characters \Chr=\ sets, by their numbers, quotes included.
var characterNames = []string{`"normal"`, `"whisper"`, `"monotone"`}

const (
	highestPitchSign = 0xffff
	highestSpeedSign = 0xffffffff
	volumeBits       = 0xffff
	tagNameLength    = 3
)

// injection is what Inject keeps while it reads a string.
type injection struct {
	converter            *Converter
	apply                func(kind InjectKind, value uint32)
	pitch, speed, volume lastNumber
}

// lastNumber is the number one of \Pit=\, \Spd=\ and \Vol=\ last read in the string, after the tag's corrections,
// which an empty value reads again. Before the tag first reads one, the original's is a variable it never set, and
// the port has none.
type lastNumber struct {
	value uint32
	set   bool
}

// scan reads value as sscanf's %u does, and reports whether there is a number.
func (last *lastNumber) scan(value []byte) bool {
	if number, read := scanNumber(value); read {
		last.value, last.set = number, true
	}
	return last.set
}

// Inject reads a string given to the engine's Inject, in code page 1252 up to its first NUL, and calls apply for
// each change it makes, in order (engine.md §4). A string that is exactly \gain=1\, \gain=2\ or \gain=3\ sets the
// gain. Otherwise each tag counts by its first three letters, ASCII case aside: \Rst...\ resets, and \Pit=\, \Spd=\,
// \Vol=\, \Chr=\ and \Vce=\ need the =. \\ is skipped, a tag of fewer than three characters is dropped, and an
// unterminated tag ends the string. The text between tags is left out.
func (converter *Converter) Inject(characters []uint16, apply func(kind InjectKind, value uint32)) {
	text := cString(converter.ansi(characters))
	if gain := slices.Index(gains, string(text)); gain >= 0 {
		apply(InjectGain, uint32(gain+1))
		return
	}
	injection := &injection{converter: converter, apply: apply}
	length := len(text)
	for position := 0; position < length; {
		if text[position] != backslash {
			position++
			continue
		}
		if at(text, position+1) == backslash {
			position += 2
			continue
		}
		closing := position + 2
		if closing >= length {
			return
		}
		for closing < length && text[closing] != backslash {
			closing++
		}
		if closing >= length {
			return
		}
		if closing-position-1 >= tagNameLength {
			injection.tag(text, position, closing)
		}
		position = closing + 1
	}
}

// tag reads the tag between position and closing. \Pit=\ takes 50 to 400 Hz, 0 standing for 50 and 65535 for 400,
// \Spd=\ 50 to 250 words a minute, 0 standing for 50 and 0xFFFFFFFF for 250, and \Vol=\ the value's low word; each
// value is digits only. \Chr=\ takes "normal", "whisper" or "monotone" in their quotes, and \Vce=\ only Speaker= as
// the first key, with the speaker's name in quotes or not, case aside.
func (injection *injection) tag(text []byte, position, closing int) {
	apply := injection.apply
	name := lowered(text[position+1 : position+1+tagNameLength])
	if name == "rst" {
		apply(InjectReset, 0)
		return
	}
	if text[position+4] != '=' {
		return
	}
	start := position + 5
	value := text[start:closing]
	digits := allDigits(value)
	switch {
	case name == "pit" && digits:
		pitch := &injection.pitch
		if !pitch.scan(value) {
			return
		}
		switch pitch.value {
		case 0:
			pitch.value = lowestPitch
		case highestPitchSign:
			pitch.value = highestPitch
		}
		if pitch.value >= lowestPitch && pitch.value <= highestPitch {
			apply(InjectPitch, pitch.value)
		}
	case name == "spd" && digits:
		speed := &injection.speed
		if !speed.scan(value) {
			return
		}
		switch speed.value {
		case 0:
			speed.value = lowestSpeed
		case highestSpeedSign:
			speed.value = highestSpeed
		}
		if speed.value >= lowestSpeed && speed.value <= highestSpeed {
			apply(InjectSpeed, speed.value)
		}
	case name == "vol" && digits:
		volume := &injection.volume
		if volume.scan(value) {
			volume.value &= volumeBits
			apply(InjectVolume, volume.value)
		}
	case name == "chr":
		if character := slices.Index(characterNames, lowered(field(text, start))); character >= 0 {
			apply(InjectCharacter, uint32(character))
		}
	case name == "vce":
		equals := start
		for equals < closing && text[equals] != '=' {
			equals++
		}
		if equals >= closing || lowered(text[start:equals]) != "speaker" {
			return
		}
		start = equals + 1
		if text[start] == '"' {
			start++
		}
		speaker := field(text, start)
		if length := len(speaker); length > 0 && speaker[length-1] == '"' {
			speaker = speaker[:length-1]
		}
		for index, mode := range injection.converter.modes.Modes {
			if lowered([]byte(mode.Speaker)) == lowered(speaker) {
				apply(InjectVoice, uint32(index))
				return
			}
		}
	}
}
