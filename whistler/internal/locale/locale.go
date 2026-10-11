package locale

// TagKey is what lstrcmpiA compares of a tag keyword or name, on Windows with code page 1252, against the
// engine's ASCII literals: soft hyphens (0xAD) don't count, 0xDF (sharp s) is "ss", and ASCII case is
// ignored (text.md §3.3).
func TagKey(name []byte) string {
	key := make([]byte, 0, len(name))
	for _, character := range name {
		switch {
		case character == 0xAD:
		case character == 0xDF:
			key = append(key, 's', 's')
		case character >= 'A' && character <= 'Z':
			key = append(key, character+'a'-'A')
		default:
			key = append(key, character)
		}
	}
	return string(key)
}
