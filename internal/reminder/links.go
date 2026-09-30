package reminder

import (
	"regexp"
	"strings"

	"aex/internal/browser"
)

// Part is a piece of a reminder's message: text, or a link (Link set) showing Text.
type Part struct {
	Text string
	Link string
}

// A message's links: "[label](link)" or a bare link, each http(s):// or aex+<browser>:// (see
// internal/browser). One that is not a web link stays text.
var linkRE = regexp.MustCompile(`(?i)\[([^\]\n]+)\]\(((?:https?|aex\+[a-z0-9-]+)://[^\s)]+)\)|(?:https?|aex\+[a-z0-9-]+)://[^\s<>"]+`)

// Parts splits the message into text and links.
func (r Reminder) Parts() []Part {
	var parts []Part
	add := func(p Part) {
		if p.Text == "" {
			return
		}
		if n := len(parts); n > 0 && p.Link == "" && parts[n-1].Link == "" {
			parts[n-1].Text += p.Text
			return
		}
		parts = append(parts, p)
	}
	msg, at := r.Message, 0
	for _, m := range linkRE.FindAllStringSubmatchIndex(msg, -1) {
		start, end := m[0], m[1]
		text, link := msg[start:end], msg[start:end]
		if m[2] >= 0 {
			text, link = msg[m[2]:m[3]], msg[m[4]:m[5]]
		} else {
			// Punctuation after a bare link ends the sentence, not the link.
			trimmed := strings.TrimRight(link, `.,;:!?'`)
			if strings.HasSuffix(trimmed, ")") && !strings.Contains(trimmed, "(") {
				trimmed = strings.TrimSuffix(trimmed, ")")
			}
			end = start + len(trimmed)
			text, link = trimmed, trimmed
		}
		if _, _, err := browser.Parse(link); err != nil {
			continue
		}
		add(Part{Text: msg[at:start]})
		add(Part{Text: text, Link: link})
		at = end
	}
	add(Part{Text: msg[at:]})
	return parts
}
