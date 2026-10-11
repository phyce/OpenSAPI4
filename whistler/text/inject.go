package text

// ReadInject reads an Inject string (interface.md §7). It converts it to code page 1252 as a text, and
// gives apply each \Pit=n\, \Spd=n\, \Vol=n\ (ControlPitch, ControlSpeed, ControlVolume) and \Rst\
// (ControlReset) in turn. It stops at a character outside a tag other than a space, at any other tag or
// value, at a value apply refuses, and after \Rst\. After each tag one character is skipped unread, as the
// original skips the closing backslash.
func (preprocessor *Preprocessor) ReadInject(tags []uint16, apply func(kind ControlKind, value uint32) bool) {
	ansi := append(preprocessor.ToANSI(tags), 0)
	position := 0
	for position < len(ansi) && ansi[position] != 0 {
		if ansi[position] == space {
			position++
			continue
		}
		if ansi[position] != backslash {
			return
		}
		cursor := &tagCursor{text: ansi, position: position + 1}
		tag, _, _ := cursor.next(tokenSize)
		switch tag {
		case tagPitch, tagSpeed, tagVolume:
			if kind, _, _ := cursor.next(tokenSize); kind != tokenEquals {
				return
			}
			kind, _, value := cursor.next(tokenSize)
			if kind != tokenNumber || !apply(ControlKind(tag), value) {
				return
			}
		case tagReset:
			apply(ControlReset, 0)
			return
		default:
			return
		}
		position = cursor.position + 1
	}
}
