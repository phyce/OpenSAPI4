package units

import "strings"

type tagForm int

const (
	numberTag tagForm = iota + 1
	quotedTag
	bareTag
)

type tagKind struct {
	form tagForm
	kind ControlKind
}

// tagKinds are the keywords the voice uses. It passes the others on unused, as it does a tag it can't read.
var tagKinds = map[string]tagKind{
	"chr": {quotedTag, Character}, "wrd": {numberTag, WordPosition}, "mrk": {numberTag, Bookmark}, "pau": {numberTag, Pause},
	"rst": {bareTag, Reset}, "spd": {numberTag, Speed}, "vol": {numberTag, Volume},
}

// ParseTag reads a tag record's text as the voice does (units.md §10). A tag it can't read is Unparsed.
func ParseTag(text string) Control {
	unparsed := Control{Kind: Unparsed}
	if !strings.HasPrefix(text, `\`) {
		return unparsed
	}
	body := text[1:]
	end := strings.IndexAny(body, `,:;=\<`)
	if end < 0 {
		end = len(body)
	}
	known, found := tagKinds[lowercase(body[:end])]
	if !found {
		return unparsed
	}
	rest := body[end:]
	switch known.form {
	case numberTag:
		if len(rest) < 2 || rest[0] != '=' || !isDigit(rest[1]) {
			return unparsed
		}
		var value uint32
		position := 1
		for ; position < len(rest) && isDigit(rest[position]); position++ {
			value = value*10 + uint32(rest[position]-'0')
		}
		if !strings.HasPrefix(rest[position:], `\`) {
			return unparsed
		}
		return Control{Kind: known.kind, Value: value}
	case quotedTag:
		if !strings.HasPrefix(rest, `="`) {
			return unparsed
		}
		closing := strings.IndexByte(rest[2:], '"') + 2
		if closing < 2 || !strings.HasPrefix(rest[closing+1:], `\`) {
			return unparsed
		}
		return Control{Kind: known.kind, Name: rest[1 : closing+1]}
	}
	if !strings.Contains(rest, `\`) {
		return unparsed
	}
	return Control{Kind: known.kind}
}

func isDigit(character byte) bool {
	return character >= '0' && character <= '9'
}
