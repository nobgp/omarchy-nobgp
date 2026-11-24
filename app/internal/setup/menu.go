package setup

import (
	"errors"
	"strings"
)

const (
	blockBegin = "  // >>> nobgp-omarchy: managed by `nobgp-omarchy setup`; edits between these lines are replaced"
	blockEnd   = "  // <<< nobgp-omarchy"
)

// MergeMenu puts entries into an Omarchy menu extension file (JSONC) inside
// a marked block, so running setup again replaces our rows and never
// touches the user's own. existing == "" means the file does not exist yet.
func MergeMenu(existing, entries string) (string, error) {
	block := blockBegin + "\n" + strings.TrimRight(entries, "\n") + "\n" + blockEnd

	if strings.TrimSpace(existing) == "" {
		return "{\n" + block + "\n}\n", nil
	}
	if b, e, ok := findBlock(existing); ok {
		return existing[:b] + block + existing[e:], nil
	}

	closeAt, lastSig, err := scanJSONC(existing)
	if err != nil {
		return "", err
	}
	head := existing[:closeAt]
	// The member before ours needs a comma, unless the object is empty or
	// already ends in one (Omarchy's parser accepts trailing commas).
	if lastSig >= 0 && existing[lastSig] != '{' && existing[lastSig] != ',' {
		head = head[:lastSig+1] + "," + head[lastSig+1:]
	}
	head = strings.TrimRight(head, " \t\n")
	return head + "\n\n" + block + "\n" + existing[closeAt:], nil
}

// RemoveMenu takes our block out again.
func RemoveMenu(existing string) (string, bool) {
	b, e, ok := findBlock(existing)
	if !ok {
		return existing, false
	}
	// Drop the block's own line break as well.
	if e < len(existing) && existing[e] == '\n' {
		e++
	}
	return existing[:b] + existing[e:], true
}

func findBlock(s string) (int, int, bool) {
	b := strings.Index(s, blockBegin)
	if b < 0 {
		return 0, 0, false
	}
	rel := strings.Index(s[b:], blockEnd)
	if rel < 0 {
		return 0, 0, false
	}
	return b, b + rel + len(blockEnd), true
}

// scanJSONC finds the brace that closes the top-level object and the last
// significant character before it, skipping strings and // and /* */
// comments.
func scanJSONC(s string) (closeAt, lastSig int, err error) {
	depth := 0
	lastSig = -1
	sawOpen := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '/' && i+1 < len(s) && s[i+1] == '/':
			for i < len(s) && s[i] != '\n' {
				i++
			}
			continue
		case c == '/' && i+1 < len(s) && s[i+1] == '*':
			end := strings.Index(s[i+2:], "*/")
			if end < 0 {
				return 0, 0, errors.New("unterminated comment in menu extension")
			}
			i += 2 + end + 1
			continue
		case c == '"':
			i++
			for i < len(s) && s[i] != '"' {
				if s[i] == '\\' {
					i++
				}
				i++
			}
			if i >= len(s) {
				return 0, 0, errors.New("unterminated string in menu extension")
			}
			lastSig = i
			continue
		case c == '{' || c == '[':
			if c == '{' && depth == 0 {
				sawOpen = true
			}
			depth++
		case c == '}' || c == ']':
			depth--
			if depth == 0 && c == '}' && sawOpen {
				return i, lastSig, nil
			}
			if depth < 0 {
				return 0, 0, errors.New("unbalanced braces in menu extension")
			}
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			continue
		}
		lastSig = i
	}
	return 0, 0, errors.New("menu extension has no top-level object")
}
