// Package compatibility does what the sapi4out and sapi4limits tools that other programs call do: how they
// read their arguments, choose a voice and set it up.
package compatibility

import (
	"fmt"
	"math"
	"math/rand/v2"

	"github.com/phyce/OpenSAPI4/whistler"
)

// Argument is a command-line argument as the tools get it.
type Argument struct {
	// ANSI is the argument as main's argv has it: on Windows in the ANSI code page, elsewhere as given.
	ANSI []byte
	// Text is the argument in UTF-16, as SAPI 4's ANSI interface converts a text with the ANSI code page.
	Text []uint16
}

// fallbackVoice is the mode the tools get for a name no mode has exactly: the one SAPI 4's Find picks when
// it is asked for nothing but a name.
const fallbackVoice = "Mary"

type Limits struct {
	Default, Minimum, Maximum uint32
}

// Voice is an engine set up as the tools set one up, with the pitch and speed limits they read from it.
type Voice struct {
	Engine       *whistler.Engine
	Pitch, Speed Limits
}

// Open makes an engine with the mode named exactly name, case included, or with Mary when no mode is. It
// reads the limits as the tools do, by setting the minimum and then the maximum pitch and speed, which stay
// set.
func Open(data *whistler.Data, name []byte) (*Voice, error) {
	mode, found := data.Mode(string(name))
	if !found {
		if mode, found = data.Mode(fallbackVoice); !found {
			return nil, fmt.Errorf("the data pack has no voice %q", fallbackVoice)
		}
	}
	engine, err := data.NewEngine(mode)
	if err != nil {
		return nil, err
	}
	voice := &Voice{Engine: engine}
	voice.Pitch.Default, voice.Speed.Default = uint32(engine.Pitch()), engine.Speed()
	engine.SetPitch(0)
	engine.SetSpeed(0)
	voice.Pitch.Minimum, voice.Speed.Minimum = uint32(engine.Pitch()), engine.Speed()
	engine.SetPitch(math.MaxUint16)
	engine.SetSpeed(math.MaxUint32)
	voice.Pitch.Maximum, voice.Speed.Maximum = uint32(engine.Pitch()), engine.Speed()
	return voice, nil
}

// Atoi reads a number as the C runtime's atoi does: after white space and a sign, the digits up to the first
// other character, and the nearest 32-bit value when the number is outside that range.
func Atoi(value []byte) int32 {
	index := 0
	for index < len(value) && isSpace(value[index]) {
		index++
	}
	negative := false
	if index < len(value) && (value[index] == '+' || value[index] == '-') {
		negative = value[index] == '-'
		index++
	}
	limit := uint64(math.MaxInt32)
	if negative {
		limit++
	}
	var magnitude uint64
	for ; index < len(value) && value[index] >= '0' && value[index] <= '9'; index++ {
		magnitude = min(magnitude*10+uint64(value[index]-'0'), limit)
	}
	if negative {
		return int32(-int64(magnitude))
	}
	return int32(magnitude)
}

func isSpace(character byte) bool {
	return character == ' ' || character >= '\t' && character <= '\r'
}

// FileName is a name of 16 random letters, as the tools give their WAV files.
func FileName() string {
	const letters = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	name := make([]byte, 16)
	for index := range name {
		name[index] = letters[rand.IntN(len(letters))]
	}
	return string(name)
}
