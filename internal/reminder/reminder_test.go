package reminder

import (
	"encoding/binary"
	"strings"
	"testing"
)

func TestMarkRoundTrip(t *testing.T) {
	r := Reminder{Title: "Stand-up", Message: "In 5 minutes\n\x07\x1b ✔"}
	mark := Mark(r)
	prefix := "\x1b]aex-" + MarkName + ";"
	if !strings.HasPrefix(mark, prefix) || !strings.HasSuffix(mark, "\x07") {
		t.Fatalf("mark %q", mark)
	}
	payload := strings.TrimSuffix(strings.TrimPrefix(mark, prefix), "\x07")
	if strings.ContainsAny(payload, "\x07\x1b") {
		t.Fatalf("payload %q has control characters", payload)
	}
	got, err := Parse(payload)
	if err != nil || got != r {
		t.Fatalf("Parse = %+v, %v; want %+v", got, err, r)
	}
}

func TestClean(t *testing.T) {
	if _, err := (Reminder{Title: "x", Message: "  "}).Clean(); err == nil {
		t.Fatal("no error for an empty message")
	}
	r, _ := Reminder{Message: strings.Repeat("é", MaxMessage+5)}.Clean()
	if n := len([]rune(r.Message)); n != MaxMessage || !strings.HasSuffix(r.Message, "…") {
		t.Fatalf("cut to %d characters", n)
	}
	if r.Heading() != "Reminder" {
		t.Fatalf("heading %q", r.Heading())
	}
}

func TestChimeIsWAV(t *testing.T) {
	w := chime()
	if string(w[:4]) != "RIFF" || string(w[8:12]) != "WAVE" || string(w[36:40]) != "data" {
		t.Fatal("not a WAV header")
	}
	if n := binary.LittleEndian.Uint32(w[40:]); int(n) != len(w)-44 {
		t.Fatalf("data size %d, file has %d", n, len(w)-44)
	}
}

func TestParts(t *testing.T) {
	r := Reminder{Message: "Open [the board](aex+brave://jira.example.com/b?x=1) or https://example.com/a. Not file:///x, (see https://go.dev)"}
	got := r.Parts()
	want := []Part{
		{Text: "Open "},
		{Text: "the board", Link: "aex+brave://jira.example.com/b?x=1"},
		{Text: " or "},
		{Text: "https://example.com/a", Link: "https://example.com/a"},
		{Text: ". Not file:///x, (see "},
		{Text: "https://go.dev", Link: "https://go.dev"},
		{Text: ")"},
	}
	if len(got) != len(want) {
		t.Fatalf("parts %q", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("part %d = %q, want %q", i, got[i], want[i])
		}
	}
}
