package input

import "github.com/phyce/OpenSAPI4/truvoice/datapack"

const (
	escape               = 0x1b
	delete               = 0x7f
	shortPhraseFreeNodes = 608
	firstHighCharacter   = 0x80
	inputFlagCount       = 7
	flagCount            = 16
	maximumByte          = 255
)

// What a command writes into the command ring: its values, or none of them, or nothing at all.
const (
	withoutValues = -1
	unwritten     = -2
)

// Put gives the escape parser the next character of the text (input.md §1.1).
func (stage *Stage) Put(character byte) {
	if !stage.Pipeline.ProsodyOff && stage.Pipeline.Free > shortPhraseFreeNodes {
		stage.Pipeline.ShortPhrase = true
	}
	switch {
	case character == '\r' || character == '\n' || character == '\t':
		character = ' '
	case character == 0 || character == delete || character < ' ' && character != escape:
		return
	}
	if character == escape {
		stage.Parsing = parsingEscape
		return
	}
	switch stage.Parsing {
	case parsingText:
		stage.putCharacter(character)
	case parsingEscape:
		stage.Parsing = parsingText
		if character == '[' {
			stage.Parsing = parsingParameters
			stage.Overflow = false
			stage.Parameter = 0
			for index := range stage.Parameters {
				stage.Parameters[index] = nothing
			}
		}
	case parsingParameters:
		stage.parseParameter(character)
	}
}

// putCharacter writes a character of the text into the command ring as its letters (input.md §1.2).
func (stage *Stage) putCharacter(character byte) {
	switch {
	case character < firstHighCharacter:
		stage.put(character)
	case character >= datapack.FirstHighCharacter:
		letters := stage.tables.HighCharacters[character-datapack.FirstHighCharacter]
		stage.put(letters[0])
		if letters[1] != 0 {
			stage.put(letters[1])
		}
	}
}

// parseParameter reads a character of a command's parameters, or its letter (input.md §1.3).
func (stage *Stage) parseParameter(character byte) {
	switch {
	case character >= '0' && character <= '9':
		if stage.Parameter >= parameterCount {
			return
		}
		parameter := &stage.Parameters[stage.Parameter]
		if *parameter == nothing {
			*parameter = 0
		}
		*parameter = *parameter*10 + int32(character-'0')
		if *parameter > maximumByte {
			stage.Overflow = true
		}
	case character == ';':
		stage.Parameter++
		if stage.Parameter < parameterCount {
			stage.Parameters[stage.Parameter] = 0
		}
	default:
		stage.command(character)
		stage.Parsing = parsingText
	}
}

// command runs a command at its letter and writes it into the command ring: the escape character, the letter, the
// number of values and the values' low bytes (input.md §1.4).
func (stage *Stage) command(letter byte) {
	count := 0
	if stage.Parameter != 0 || stage.Parameters[0] != nothing {
		count = int(stage.Parameter) + 1
	}
	if stage.Overflow || count > parameterCount {
		letter = 0
	}
	settings := &stage.Pipeline.Settings
	value := stage.Parameters[0]
	values := int32(withoutValues)
	switch letter {
	case 'A', 'D':
		bits := stage.flagBits(count, inputFlagCount)
		if letter == 'A' {
			settings.InputFlags |= bits
		} else {
			settings.InputFlags &^= bits
		}
		letter = 'A'
		value, values = stage.flagValues(settings.InputFlags)
	case 'N', 'F':
		bits := stage.flagBits(count, flagCount)
		if letter == 'N' {
			settings.Flags |= bits
		} else {
			settings.Flags &^= bits
		}
		letter = 'N'
		value, values = stage.flagValues(settings.Flags)
	case 'C':
		if count == 0 {
			stage.Pipeline.Synthesizer.Hold = false
			values = 0
		}
	case 'H':
		if count == 0 {
			stage.Pipeline.Synthesizer.Hold = true
			values = unwritten
		}
	case 'S':
		if count == 0 {
			stage.Pipeline.ResetRequested = true
			values = unwritten
		}
	case 'x':
		if count == 0 {
			stage.Pipeline.ParsedReset = true
			stage.Pipeline.Synthesizer.Hold = false
			values = 0
		}
	case 'w':
		if count == 0 {
			stage.Pipeline.DefaultSettings()
			for index := range stage.Pipeline.Cursors {
				stage.Pipeline.Cursors[index].Settings = *settings
			}
			values = unwritten
		}
	case 'I':
		if count < 2 && value < 2 {
			value = max(value, 0)
			settings.PhoneticInput = value
			values = 1
		}
	case 'P':
		if count < 2 && value < 2 {
			if value < 0 {
				value = 1
			}
			settings.Phrase = value
			values = 1
		}
	case 'V':
		if count < 2 {
			value = min(max(value, 0), datapack.VoiceCount-1)
			settings.Voice = value
			values = 1
		}
	case 'a':
		if count < 2 {
			value = min(max(value, 0), 16)
			settings.Attenuation = value
			values = 1
		}
	case 'c':
		if count < 2 {
			if value < 0 || value > 2 {
				value = 0
			}
			values = 1
		}
	case 'f':
		if count < 2 {
			value = min(max(value, 0), datapack.FilterCount-1)
			settings.Filter = value
			settings.Rate = min(settings.Rate, stage.tables.FilterRates[value])
			stage.Parameters[1] = settings.Rate
			values = 2
		}
	case 'g', 's':
		if count < 2 {
			switch {
			case value < 0:
				value = 100
			case value == 0:
				value = 1
			default:
				value = min(value, maximumByte)
			}
			values = 1
		}
	case 'i', 'n', 'z':
		if count < 5 {
			values = int32(count)
			if value < 0 {
				value, values = 1, 1
			}
		}
	case 'l':
		if value >= 0 && count < 3 && value < datapack.TrackCount {
			level := &stage.Parameters[1]
			if *level < 0 {
				*level = int32(stage.Pipeline.GeneratedLevel(int(value)))
			} else {
				*level = min(*level, stage.tables.LevelLimits[value])
			}
			stage.Pipeline.SettingsLevels[value] = byte(*level)
			values = 2
		}
	case 'p':
		if count < 2 {
			switch {
			case value < 0:
				value = 42
				settings.Pitch = 84
			case value < 25:
				value = 25
				settings.Pitch = 50
			default:
				value = min(value, 200)
				settings.Pitch = value * 2
			}
			values = 1
		}
	case 'r':
		if count < 2 {
			switch {
			case value >= 50:
				stage.Pipeline.SettingsSpeed = value
			case value > 0:
				value = 50
				stage.Pipeline.SettingsSpeed = value
			default:
				stage.Pipeline.SettingsSpeed = 150
			}
			if value > 0 {
				value = (value - 46) >> 3
			}
			value, values = stage.rate(value, false)
		}
	case 'v':
		if count < 2 {
			value, values = stage.rate(value, true)
		}
	case 't':
		if count < 4 {
			values = 3
			switch {
			case value < 0:
				value = 0
			case value > 9:
				values = withoutValues
			}
			stage.Parameters[1] = clamp(stage.Parameters[1], 100, 1, maximumByte)
			stage.Parameters[2] = clamp(stage.Parameters[2], 50, 30, 243)
		}
	}
	stage.Parameters[0] = value
	if values == unwritten || stage.room() < values+3 {
		return
	}
	stage.put(escape)
	stage.put(letter)
	stage.put(byte(values))
	for index := range max(values, 0) {
		stage.put(byte(stage.Parameters[index]))
	}
}

// flagBits are the bits of the flags numbered 1 to highest among the command's parameters.
func (stage *Stage) flagBits(count int, highest int32) uint32 {
	var bits uint32
	for _, parameter := range stage.Parameters[:count] {
		if parameter > 0 && parameter <= highest {
			bits |= 1 << (parameter - 1)
		}
	}
	return bits
}

// flagValues are a flag command's values: the flags' low byte, and their second byte as the second parameter.
func (stage *Stage) flagValues(flags uint32) (int32, int32) {
	stage.Parameters[1] = int32(flags >> 8 & 0xff)
	return int32(flags & 0xff), 2
}

// rate sets the rate index of an r or v command, and the speed of a v command.
func (stage *Stage) rate(value int32, speed bool) (int32, int32) {
	settings := &stage.Pipeline.Settings
	switch {
	case value < 0:
		value = 13
	case value > datapack.MaximumRate:
		value = datapack.MaximumRate
	}
	if settings.Filter != 0 {
		value = min(value, stage.tables.FilterRates[settings.Filter])
	}
	settings.Rate = value
	if speed {
		stage.Pipeline.SettingsSpeed = value*8 + 50
	}
	return value, 1
}

// clamp is a value below 0 made the default, or else a value kept from low to high.
func clamp(value, missing, low, high int32) int32 {
	if value < 0 {
		return missing
	}
	return min(max(value, low), high)
}
