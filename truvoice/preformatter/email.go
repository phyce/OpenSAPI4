package preformatter

import (
	"strings"

	"github.com/phyce/OpenSAPI4/truvoice/datapack"
)

const (
	shortHeader = 38
	colonSpace  = 12
	heldLimit   = 743
)

// state is where the e-mail reading is (section 3.6): outside a header block, in one, or right after one.
type state int

const (
	outside state = iota
	inBlock
	afterBlock
)

// email reads a piece as part of an e-mail message (section 3.6). It reports whether the piece is left for the
// paragraphs, and gives what is left of it.
func (address *address) email(piece string, end ending) (string, bool) {
	line := strings.TrimLeft(piece, " ")
	afterSubject := address.afterSubject
	address.afterSubject = false
	if address.holding {
		held := address.held
		address.held, address.holding = "", false
		if !colonForm(piece, true) {
			address.release(held, piece, end)
			address.join(unquoted(piece), end, false, piece, true)
			if end == textEnd {
				address.single = false
			}
			return "", false
		}
	}
	if address.namesGoOn != datapack.NoHeader {
		action := address.namesGoOn
		address.namesGoOn = datapack.NoHeader
		if !colonForm(piece, false) && !(action == datapack.DroppedHeader && blankHeaderLine(line)) {
			address.continueNames(action, piece, end)
			return "", false
		}
	}
	switch address.state {
	case inBlock:
		if address.subjectGoesOn {
			address.subjectGoesOn = false
			if blankHeaderLine(line) {
				if end == lineFeed {
					address.output.WriteString(lineEnd)
				} else {
					address.output.WriteString(paragraphEnd)
				}
				address.state = afterBlock
				return "", false
			}
			if !headerForm(piece, end) {
				address.subjectLine(piece, end)
				address.hold(piece)
				return "", false
			}
		}
		if blankHeaderLine(line) {
			address.output.WriteString(paragraphEnd)
			address.state = afterBlock
			return "", false
		}
		if name, value, action, ok := address.header(line); ok {
			address.headerLine(name, value, action, end, afterSubject)
			return "", false
		}
		if name, value, ok := address.headerName(line, true); ok {
			address.continues(name, value, datapack.DroppedHeader)
		}
		address.output.WriteString(paragraphEnd)
		address.hold(piece)
		return "", false
	case afterBlock:
		if name, value, action, ok := address.header(line); ok {
			address.headerLine(name, value, action, end, afterSubject)
			return "", false
		}
		address.state = outside
	default:
		if name, value, action, ok := address.header(line); ok {
			address.headerLine(name, value, action, end, afterSubject)
			return "", false
		}
	}
	return unquoted(piece), true
}

// headerName is the header a line is, its name right before a colon, and its value. One `>` may quote it, and in
// or right after a header block two a header quoted twice.
func (address *address) headerName(line string, twice bool) (name, value string, ok bool) {
	line = strings.TrimLeft(line, " ")
	if strings.HasPrefix(line, ">") {
		line = line[1:]
		if twice && strings.HasPrefix(line, ">") {
			if quoted, _, found := strings.Cut(line[1:], ":"); found && address.headers[quoted].QuotedTwice {
				line = line[1:]
			}
		}
	}
	name, value, found := strings.Cut(line, ":")
	if _, known := address.headers[name]; !found || !known {
		return "", "", false
	}
	return name, value, true
}

// header is the header a line is where the reading is, and what is done with it.
func (address *address) header(line string) (name, value string, action datapack.HeaderAction, ok bool) {
	name, value, ok = address.headerName(line, address.state != outside)
	if !ok {
		return "", "", datapack.NoHeader, false
	}
	rule := address.headers[name]
	names, action := rule.OutsideNames, rule.Outside
	switch address.state {
	case inBlock:
		names, action = rule.InsideNames, rule.Block
	case afterBlock:
		names, action = rule.InsideNames, rule.After
	}
	if names > 0 && address.messages >= names {
		action = datapack.NameHeader
	}
	return name, value, action, action != datapack.NoHeader
}

// headerLine writes a header line and moves the reading on (section 3.7).
func (address *address) headerLine(name, value string, action datapack.HeaderAction, end ending, afterSubject bool) {
	spaced := strings.HasPrefix(value, " ") || strings.HasPrefix(value, "\t")
	if colonsSpaced(name + ":" + value) {
		spaced = true
		value = spaceColons(value)
	}
	address.continues(name, value, action)
	inside := address.state != outside
	if address.paragraph != "" {
		address.flush()
	}
	switch action {
	case datapack.DroppedHeader, datapack.BlockHeader:
		address.output.WriteString(paragraphEnd)
	case datapack.MessageHeader:
		address.output.WriteString(paragraphEnd + address.messageStart + ". " + paragraphEnd)
	case datapack.AuthorHeader:
		author, fromAddress := people(authorDots(strings.ReplaceAll(value, "\t", " ")))
		text := name + ":"
		if author != "" {
			text += " " + author
		}
		if !strings.HasSuffix(text, ".") {
			text += "."
		}
		if !afterSubject {
			address.output.WriteString(paragraphEnd)
		}
		address.output.WriteString(text)
		if end == lineFeed && !fromAddress {
			address.output.WriteString(lineEnd)
		} else {
			address.output.WriteString(paragraphEnd)
		}
	default:
		var whole string
		var fromAddress bool
		switch action {
		case datapack.SenderHeader:
			value, fromAddress = person(value)
		case datapack.NameHeader:
			value, fromAddress = people(value)
		default:
			whole = clean(value)
			value = strings.TrimRight(whole, " ")
			fromAddress = action == datapack.DateHeader
		}
		ended := action == datapack.SubjectHeader && inside && address.author
		if ended && end == lineFeed {
			address.paragraph = joined(name + ": " + whole)
		} else {
			text := name + ":"
			if value != "" && spaced || ended {
				text += " "
			}
			text += value
			short := end == lineFeed && !fromAddress && len(text) < shortHeader
			switch {
			case short && strings.HasSuffix(text, "."):
				text += "  "
			case short:
				text += ".  "
			case !strings.HasSuffix(text, "."):
				text += "."
			}
			address.paragraph = text
		}
		address.flush()
	}
	address.move(name, action, inside, afterSubject)
}

// move is where the reading goes after a header line (section 3.6).
func (address *address) move(name string, action datapack.HeaderAction, inside, afterSubject bool) {
	author := action == datapack.AuthorHeader && !afterSubject
	starts := action == datapack.BlockHeader || action == datapack.MessageHeader || action == datapack.SenderHeader
	switch {
	case author:
		address.messages++
		address.author = true
	case starts && !inside:
		address.messages++
		if address.headers[name].ClearsAuthor {
			address.author = false
		}
	}
	switch {
	case action == datapack.SubjectHeader:
		address.afterSubject = true
		address.subjectGoesOn = inside && !address.author
		if address.subjectGoesOn {
			address.state = inBlock
		} else {
			address.state = outside
		}
	case inside || starts || author:
		address.state = inBlock
	}
}

// continues is whether an Author: or names value goes on in the next line, or the value of a header that
// continues read as neither: it ends in a comma.
func (address *address) continues(name, value string, action datapack.HeaderAction) {
	if !strings.HasSuffix(strings.TrimRight(value, " \t"), ",") {
		return
	}
	switch {
	case action == datapack.AuthorHeader || action == datapack.NameHeader:
		address.namesGoOn = action
	case address.headers[name].Continues:
		address.namesGoOn = datapack.DroppedHeader
	}
}

// continueNames writes a line that goes on with an Author: or names value (section 3.6).
func (address *address) continueNames(action datapack.HeaderAction, piece string, end ending) {
	if strings.HasSuffix(strings.TrimRight(piece, " \t"), ",") {
		address.namesGoOn = action
	}
	if action == datapack.DroppedHeader {
		address.output.WriteString(paragraphEnd)
		return
	}
	text, fromAddress := continued(piece)
	if !strings.HasSuffix(text, ".") {
		text += "."
	}
	if action == datapack.AuthorHeader {
		address.output.WriteString(paragraphEnd + text)
		if end == lineFeed && !fromAddress {
			address.output.WriteString(lineEnd)
		} else {
			address.output.WriteString(paragraphEnd)
		}
		address.state = inBlock
		return
	}
	address.paragraph = text
	address.flush()
}

// subjectLine writes the line that goes on after a Subject: line in a header block (section 3.6): no blank line,
// no colon spaces, and as the text's last line no dot, and a single character (the space after a quote's > counted)
// as it is.
func (address *address) subjectLine(piece string, end ending) {
	kept := unquoted(piece)
	if end == textEnd {
		text := collapse(strings.ReplaceAll(strings.TrimLeft(kept, " "), "\t", " "))
		bare := strings.TrimLeft(strings.ReplaceAll(piece, "\t", " "), " ")
		if len(strings.TrimRight(strings.TrimPrefix(bare, ">"), " ")) <= 1 {
			if trimmed := strings.Trim(text, " "); trimmed != "" {
				text = trimmed
			}
			address.output.WriteString(text + paragraphEnd)
			return
		}
		address.paragraph = strings.Trim(text, " ")
	} else {
		address.join(kept, end, false, kept, false)
	}
	if address.paragraph != "" {
		address.flush()
	}
}

// hold keeps a line of a header block that ends in a comma and isn't a header line for the next one (section 3.6).
func (address *address) hold(piece string) {
	if strings.HasSuffix(piece, ",") && !colonForm(piece, true) {
		address.held, address.holding = piece, true
	}
}

// release ends the header block for the line after a held line, and writes the held line unless the two are too
// long together: a cut piece counts without the spaces it's cut at, a line with its line feed.
func (address *address) release(held, piece string, end ending) {
	address.state = outside
	address.join("", lineFeed, false, "", false)
	following := measured(piece)
	switch end {
	case cutEnding:
		following = measured(strings.TrimRight(piece, " "))
	case lineFeed:
		following++
	}
	if measured(held)+following >= heldLimit {
		return
	}
	text := clean(unquoted(held))
	if colonsSpaced(held) {
		text = spaceColons(text)
	}
	address.paragraph += text + ".  "
	address.longParagraph += text + ".  "
	address.rawSize += len(text) + 3
	address.lineStart = len(address.paragraph)
}

// authorDots makes each dot followed by k spaces, k two or more, k dots and a space.
func authorDots(value string) string {
	var output strings.Builder
	for index := 0; index < len(value); index++ {
		output.WriteByte(value[index])
		if value[index] != '.' {
			continue
		}
		run := spaceRun(value, index+1)
		if spaces := run - index - 1; spaces >= 2 {
			output.WriteString(strings.Repeat(".", spaces-1) + " ")
			index = run - 1
		}
	}
	return output.String()
}

func blankHeaderLine(line string) bool {
	return line == "" || line == ">"
}

// colonForm is whether a line reads as a header line in a header block: a colon after two characters or more, one
// `>` before them left out; unless spaced, no space or tab before the colon.
func colonForm(piece string, spaced bool) bool {
	line := strings.TrimPrefix(strings.TrimLeft(piece, " "), ">")
	colon := strings.IndexByte(line, ':')
	return colon >= 2 && (spaced || !strings.ContainsAny(line[:colon], " \t"))
}

// headerForm is whether the line after a Subject: line in a header block looks like a header: it has a colon and a
// space once its tabs are spaces and its colons are spaced. The spaces the text's last line ends with don't count.
func headerForm(piece string, end ending) bool {
	text := clean(unquoted(piece))
	if end == textEnd {
		text = strings.TrimRight(text, " ")
	}
	if colonsSpaced(piece) {
		text = spaceColons(text)
	}
	return strings.Contains(text, ": ")
}

// colonsSpaced is whether a line's colons get a space after them (section 3.2): its first colon has no space or tab
// before it or right after it, and the line has a space or a tab after it, or the colon is from its 2nd to its
// 12th character. Leading spaces and one `>` don't count.
func colonsSpaced(read string) bool {
	line := strings.TrimPrefix(strings.TrimLeft(read, " "), ">")
	colon := strings.IndexByte(line, ':')
	if colon < 0 || strings.ContainsAny(line[:min(colon+2, len(line))], " \t") {
		return false
	}
	return strings.ContainsAny(line[colon:], " \t") || colon >= 1 && colon < colonSpace
}

// spaceColons puts a space after each colon that no space or tab follows.
func spaceColons(line string) string {
	var spaced strings.Builder
	for index := 0; index < len(line); index++ {
		spaced.WriteByte(line[index])
		if line[index] == ':' && (index+1 == len(line) || line[index+1] != ' ' && line[index+1] != '\t') {
			spaced.WriteByte(' ')
		}
	}
	return spaced.String()
}

// measured is a line's length as the held line's limit counts it: without leading spaces and one `>`, its tabs
// spaces, its runs of spaces one but after a dot, its colons' spaces counted.
func measured(line string) int {
	spaced := colonsSpaced(line)
	line = strings.TrimPrefix(strings.TrimLeft(line, " "), ">")
	if spaced {
		line = spaceColons(line)
	}
	return len(collapse(strings.ReplaceAll(line, "\t", " ")))
}

// joined is a line as a paragraph's line ends (section 3.3), with a dot after a long one.
func joined(line string) string {
	short := strings.HasSuffix(line, ":") || len(line) < shortLine
	switch {
	case short && strings.HasSuffix(line, "."):
		return line + "  "
	case short:
		return line + ".  "
	case endsInDotSpaces(line):
		return line + "."
	}
	return strings.TrimRight(line, " ") + " ."
}

// unquoted is a quoted line without its `>` and a space after it.
func unquoted(piece string) string {
	line := strings.TrimLeft(piece, " ")
	if !strings.HasPrefix(line, ">") {
		return piece
	}
	return strings.TrimPrefix(line[1:], " ")
}
