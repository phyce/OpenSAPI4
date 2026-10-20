package text

import (
	"slices"

	"github.com/phyce/OpenSAPI4/truvoice/preformatter"
)

const (
	// What TextData puts around a text the pre-formatter formats in the address or e-mail context: flags 7 and 8
	// (spelling words of capitals and abbreviations) off, then on again.
	formattedStart = "\x1b[7F\x1b[8F"
	formattedEnd   = "\x1b[7N\x1b[8N"
)

// The characters 128 and up a text in a tagged text or a context keeps: ’ £ ¥ © ®.
var keptHighCharacters = []byte{0x92, 0xa3, 0xa5, 0xa9, 0xae}

// applyContexts reads a converted text's \Ctx="..."\ tags (engine.md §1.4). Each ends a segment, which it writes as
// the context before it has it, and sets the engine's context: "email" and "e-mail" 4, "spreadsheet" 6, "address" 1,
// anything else 0, written as ESC[nX. It drops every other tag and writes \\ as \. An unterminated tag ends the text.
func (converter *Converter) applyContexts(text []byte, characterSet CharacterSet) []byte {
	length := len(text)
	var applied, segment []byte
	flush := func() {
		if characterSet == Text {
			applied = append(applied, converter.preFormat(blank(segment))...)
		} else {
			applied = append(applied, segment...)
		}
		segment = nil
	}
	for position := 0; position < length; {
		if text[position] != backslash {
			segment = append(segment, text[position])
			position++
			continue
		}
		if at(text, position+1) == backslash {
			segment = append(segment, backslash)
			position += 2
			continue
		}
		closing := position + 2
		for closing < length && text[closing] != backslash {
			closing++
		}
		if length <= closing {
			break
		}
		if at(text, position+4) == '=' && lowered(text[position+1:position+4]) == "ctx" {
			flush()
			context := NoContext
			switch lowered(field(text, position+5)) {
			case `"e-mail"`, `"email"`:
				context = EmailContext
			case `"spreadsheet"`:
				context = SpreadsheetContext
			case `"address"`:
				context = AddressContext
			}
			applied = append(applied, escape, '[', byte(context)+'0', 'X')
			converter.Context = context
		}
		position = closing + 1
	}
	flush()
	return applied
}

// preFormat writes a text in code page 1252 as the context has it: pre-formatted in the spreadsheet context, and
// between formattedStart and formattedEnd in the address and e-mail contexts. The pre-formatter of those two isn't
// ported: the port leaves the text as it is there.
func (converter *Converter) preFormat(text []byte) []byte {
	switch converter.Context {
	case AddressContext, EmailContext:
		return slices.Concat([]byte(formattedStart), text, []byte(formattedEnd))
	case SpreadsheetContext:
		return preformatter.Spreadsheet(&converter.preFormatter.Spreadsheet, text)
	}
	return text
}

// blank turns a text's characters 128 and up into spaces, but those it keeps.
func blank(text []byte) []byte {
	blanked := slices.Clone(text)
	for index, character := range blanked {
		if character >= 0x80 && !slices.Contains(keptHighCharacters, character) {
			blanked[index] = ' '
		}
	}
	return blanked
}
