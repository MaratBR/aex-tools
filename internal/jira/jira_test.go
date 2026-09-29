package jira

import "testing"

func TestParseKey(t *testing.T) {
	for in, want := range map[string]string{
		"CM-7042":   "CM-7042",
		" cm-7042 ": "CM-7042",
		"https://clearmechanic.atlassian.net/browse/CM-7042":                                           "CM-7042",
		"https://clearmechanic.atlassian.net/browse/CM-7042?focusedCommentId=1":                        "CM-7042",
		"https://clearmechanic.atlassian.net/jira/software/projects/CM/boards/1?selectedIssue=CM-7042": "CM-7042",
		"CM-":     "",
		"CM 7042": "",
		"https://clearmechanic.atlassian.net/browse/": "",
		"https://x/browse/CM-7042 extra":              "",
	} {
		got, ok := ParseKey(in)
		if !ok {
			got = ""
		}
		if got != want {
			t.Errorf("ParseKey(%q) = %q, want %q", in, got, want)
		}
	}
}
