package evaluate

import (
	"regexp"
	"strings"
)

var (
	fenceLinePattern = regexp.MustCompile(`^ {0,3}([` + "`" + `]{3,}|~{3,})(.*)$`)
	blockLinePattern = regexp.MustCompile(`^ {0,3}\S`)
)

func splitLines(body string) []string {
	lines := make([]string, 0, 16)
	start := 0
	for i := 0; i < len(body); {
		if body[i] == '\r' {
			lines = append(lines, body[start:i])
			i++
			if i < len(body) && body[i] == '\n' {
				i++
			}
			start = i
			continue
		}
		if body[i] == '\n' {
			lines = append(lines, body[start:i])
			i++
			start = i
			continue
		}
		i++
	}
	return append(lines, body[start:])
}

func fenceRemainderIsSpaceTab(rest string) bool {
	for i := range rest {
		if rest[i] != ' ' && rest[i] != '\t' {
			return false
		}
	}
	return true
}

func unfencedLines(body string) []string {
	var (
		out            []string
		fenceCharacter byte
		fenceLength    int
	)
	for _, line := range splitLines(body) {
		fenceMatch := fenceLinePattern.FindStringSubmatch(line)
		if fenceCharacter != 0 {
			if fenceMatch != nil {
				marker := fenceMatch[1]
				if marker[0] == fenceCharacter && len(marker) >= fenceLength && fenceRemainderIsSpaceTab(fenceMatch[2]) {
					fenceCharacter = 0
					fenceLength = 0
				}
			}
			continue
		}
		if fenceMatch != nil {
			marker := fenceMatch[1]
			if marker[0] == '`' && strings.Contains(fenceMatch[2], "`") {
				out = append(out, line)
				continue
			}
			fenceCharacter = marker[0]
			fenceLength = len(marker)
			continue
		}
		out = append(out, line)
	}
	return out
}

func markdownBlockLines(body string) []string {
	var out []string
	for _, line := range unfencedLines(body) {
		if blockLinePattern.MatchString(line) {
			out = append(out, line)
		}
	}
	return out
}

func validateHeadingContract(body string, definition map[string]any, label string) []string {
	_, _, _ = body, definition, label
	return nil
}
