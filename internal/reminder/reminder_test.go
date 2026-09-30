package reminder

import (
	"encoding/binary"
	"strings"
	"sync"
	"testing"
	"time"
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
		t.Fatalf("parts %+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("part %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestPartsCloseMark(t *testing.T) {
	r := Reminder{Message: "[Join](!aex+brave://meet.example.com/x) !https://a.example Hi!https://b.example [Keep](https://c.example)"}
	got := r.Parts()
	want := []Part{
		{Text: "Join", Link: "aex+brave://meet.example.com/x", Close: true},
		{Text: " "},
		{Text: "https://a.example", Link: "https://a.example", Close: true},
		{Text: " Hi!"},
		{Text: "https://b.example", Link: "https://b.example"},
		{Text: " "},
		{Text: "Keep", Link: "https://c.example"},
	}
	if len(got) != len(want) {
		t.Fatalf("parts %+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("part %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestAnnounce(t *testing.T) {
	oldGap, oldEvery, oldFor := chimeGap, urgentEvery, urgentFor
	chimeGap, urgentEvery, urgentFor = 5*time.Millisecond, 40*time.Millisecond, 95*time.Millisecond
	t.Cleanup(func() { chimeGap, urgentEvery, urgentFor = oldGap, oldEvery, oldFor })

	var mu sync.Mutex
	plays, quiets := 0, 0
	play := func() { mu.Lock(); plays++; mu.Unlock() }
	quiet := func() { mu.Lock(); quiets++; mu.Unlock() }
	count := func() (int, int) { mu.Lock(); defer mu.Unlock(); return plays, quiets }

	announce(false, nil, play, quiet)
	if p, _ := count(); p != 1 {
		t.Fatalf("not urgent: %d chimes", p)
	}

	// Urgent, never closed: twice at 0, 40 and 80 ms, then urgentFor ends it.
	plays = 0
	announce(true, make(chan struct{}), play, quiet)
	time.Sleep(200 * time.Millisecond)
	if p, q := count(); p != 6 || q != 0 {
		t.Fatalf("urgent for its time: %d chimes, %d quiets; want 6, 0", p, q)
	}

	// Closed after the first pair: it stops and goes quiet.
	plays = 0
	stop := make(chan struct{})
	announce(true, stop, play, quiet)
	time.Sleep(20 * time.Millisecond)
	close(stop)
	time.Sleep(60 * time.Millisecond)
	if p, q := count(); p != 2 || q != 1 {
		t.Fatalf("closed: %d chimes, %d quiets; want 2, 1", p, q)
	}
}
