package reminder

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"aex/internal/browser"
)

// Part is a piece of a reminder's message: text, or a link (Link set) showing Text. Close is a
// link that also closes the reminder once opened.
type Part struct {
	Text  string
	Link  string
	Close bool
}

// CloseMark before a link makes it close the reminder once opened: "[Join](!https://…)", or a
// bare "!https://…" (at the start or after a space, so "Hi!https://…" is a plain link).
const CloseMark = "!"

// A message's links: "[label](link)" or a bare link, each http(s):// or aex+<browser>:// (see
// internal/browser), maybe with CloseMark before the link. One that is not a web link stays text.
var linkRE = regexp.MustCompile(`(?i)\[([^\]\n]+)\]\((!?)((?:https?|aex\+[a-z0-9-]+)://[^\s)]+)\)|(!?)((?:https?|aex\+[a-z0-9-]+)://[^\s<>"]+)`)

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
		var p Part
		if m[2] >= 0 {
			p = Part{Text: msg[m[2]:m[3]], Link: msg[m[6]:m[7]], Close: m[5] > m[4]}
		} else {
			link := msg[m[10]:m[11]]
			// Punctuation after a bare link ends the sentence, not the link.
			trimmed := strings.TrimRight(link, `.,;:!?'`)
			if strings.HasSuffix(trimmed, ")") && !strings.Contains(trimmed, "(") {
				trimmed = strings.TrimSuffix(trimmed, ")")
			}
			end = m[10] + len(trimmed)
			p = Part{Text: trimmed, Link: trimmed}
			if m[9] > m[8] {
				// The mark counts at the start or after a space; else it is text before the link.
				if before, _ := utf8.DecodeLastRuneInString(msg[:start]); start == 0 || unicode.IsSpace(before) {
					p.Close = true
				} else {
					start = m[10]
				}
			}
		}
		if _, _, err := browser.Parse(p.Link); err != nil {
			continue
		}
		add(Part{Text: msg[at:start]})
		add(p)
		at = end
	}
	add(Part{Text: msg[at:]})
	return parts
}
