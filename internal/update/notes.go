package update

import (
	"regexp"
	"strings"
)

var mdNoise = regexp.MustCompile("[*_`]+|\\[([^\\]]*)\\]\\([^)]*\\)")

// WhatsNew turns release notes into at most four short bullets. more is true
// only when the notes contain something the bullets leave out.
func WhatsNew(body string) (bullets []string, more bool) {
	const maxBullets, maxLen = 4, 140
	for _, line := range strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n") {
		t := strings.TrimSpace(line)
		switch {
		case t == "" || strings.HasPrefix(t, "#") || strings.HasPrefix(t, "<!--"):
			continue
		case strings.HasPrefix(t, "- ") || strings.HasPrefix(t, "* "):
			if strings.HasPrefix(line, "  ") || strings.HasPrefix(line, "\t") {
				more = true // nested detail
				continue
			}
			text := strings.TrimSpace(mdNoise.ReplaceAllString(t[2:], "$1"))
			if text == "" {
				continue
			}
			if len(bullets) == maxBullets {
				more = true
				continue
			}
			if r := []rune(text); len(r) > maxLen {
				text = strings.TrimSpace(string(r[:maxLen-1])) + "…"
				more = true
			}
			bullets = append(bullets, text)
		default:
			more = true
		}
	}
	return bullets, more
}
