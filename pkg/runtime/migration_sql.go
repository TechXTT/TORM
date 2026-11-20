package runtime

import (
	"fmt"
	"regexp"
	"strings"
)

var dollarQuote = regexp.MustCompile(`^\$(?:[A-Za-z_][A-Za-z0-9_]*)?\$`)

func wordByte(ch byte) bool {
	return ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || ch == '_' || ch == '$' || ch >= 128
}

func validateSQL(input string) error {
	var first bool = true
	var count int

	for i := 0; i < len(input); {
		switch {
		case input[i] == ' ' || input[i] == '\r' || input[i] == '\n' || input[i] == '\t' || input[i] == '\f' || input[i] == '\v':
			i++
		case strings.HasPrefix(input[i:], "--"):
			end := strings.IndexByte(input[i:], '\n')
			if end == -1 {
				i = len(input)
			} else {
				i += end + 1
			}
		case strings.HasPrefix(input[i:], "/*"):
			i += 2
			depth := 1
			for i < len(input) && depth != 0 {
				switch {
				case strings.HasPrefix(input[i:], "/*"):
					depth++
					i += 2
				case strings.HasPrefix(input[i:], "*/"):
					depth--
					i += 2
				default:
					i++
				}
			}
			if depth != 0 {
				return fmt.Errorf("unterminated SQL comment")
			}
		case input[i] == ';':
			first = true
			i++
		case input[i] == '\'' || input[i] == '"':
			quote := input[i]
			escaped := quote == '\'' && i > 0 && (input[i-1] == 'E' || input[i-1] == 'e') && (i == 1 || !wordByte(input[i-2]))
			i++
			closed := false
			for i < len(input) {
				if escaped && input[i] == '\\' {
					i += 2
					continue
				}
				if input[i] == quote {
					i++
					if i < len(input) && input[i] == quote {
						i++
						continue
					}
					closed = true
					break
				}
				i++
			}
			if !closed {
				return fmt.Errorf("unterminated SQL literal or identifier")
			}
		case input[i] == '$':
			tag := dollarQuote.FindString(input[i:])
			if tag == "" {
				i++
				continue
			}
			i += len(tag)
			end := strings.Index(input[i:], tag)
			if end == -1 {
				return fmt.Errorf("unterminated dollar-quoted SQL body")
			}
			i += end + len(tag)
		case wordByte(input[i]):
			start := i
			for i < len(input) && wordByte(input[i]) {
				i++
			}
			if first {
				switch keyword := strings.ToUpper(input[start:i]); keyword {
				case "ABORT", "BEGIN", "COMMIT", "END", "PREPARE", "RELEASE", "ROLLBACK", "SAVEPOINT", "START", "SET", "RESET", "DISCARD":
					return fmt.Errorf("%s is managed by TORM and cannot appear as a migration statement", keyword)
				}
				count++
				first = false
			}
		default:
			i++
		}
	}
	if count == 0 {
		return fmt.Errorf("migration contains no SQL statements")
	}
	return nil
}
