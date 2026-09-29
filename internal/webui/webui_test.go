package webui

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"

	"aex/internal/ui"
)

// collector gathers emitted output.
type collector struct {
	mu sync.Mutex
	b  strings.Builder
}

func (c *collector) emit(s string)  { c.mu.Lock(); c.b.WriteString(s); c.mu.Unlock() }
func (c *collector) String() string { c.mu.Lock(); defer c.mu.Unlock(); return c.b.String() }

func TestStreamSyncWaitsForEarlierOutput(t *testing.T) {
	r, w := io.Pipe()
	var c collector
	s := newStream(w, c.emit)
	go s.pump(r)
	for i := range 50 {
		fmt.Fprintf(w, "line %d ✔\n", i)
		s.Sync()
		if want := fmt.Sprintf("line %d ✔\n", i); !strings.HasSuffix(c.String(), want) {
			t.Fatalf("after Sync %d: output ends %q, want %q", i, tail(c.String()), want)
		}
	}
	if strings.Contains(c.String(), "\x1b") {
		t.Fatalf("sync marks leaked into output: %q", c.String())
	}
}

func TestStreamForwardKeepsCutOffParts(t *testing.T) {
	var c collector
	s := newStream(io.Discard, c.emit)
	reached := make(chan struct{})
	s.reached[7] = reached
	input := "héllo " + syncMark + "7\x07wörld\n"
	// Feed it one byte at a time: every mark and character is cut off at some point.
	var pending []byte
	for i := range len(input) {
		pending = s.forward(append(pending, input[i]))
	}
	if got, want := c.String(), "héllo wörld\n"; got != want {
		t.Fatalf("output %q, want %q", got, want)
	}
	select {
	case <-reached:
	default:
		t.Fatal("sync mark 7 not reported")
	}
}

func tail(s string) string {
	if len(s) > 40 {
		return s[len(s)-40:]
	}
	return s
}

// testApp answers each prompt with the next of answers, recording the prompts.
func testApp(t *testing.T, answers ...answer) (*App, *[]prompt) {
	a := &App{waiting: map[int]chan answer{}, fields: map[int]ui.Field{}}
	var prompts []prompt
	a.emit = func(event string, data any) {
		if event != "prompt" {
			return
		}
		p := data.(prompt)
		prompts = append(prompts, p)
		if len(answers) == 0 {
			t.Fatalf("unexpected prompt %+v", p)
		}
		ans := answers[0]
		answers = answers[1:]
		go a.Answer(p.ID, ans.value, ans.cancelled)
	}
	return a, &prompts
}

func TestInputAsksAgainUntilValid(t *testing.T) {
	a, prompts := testApp(t, answer{value: "  "}, answer{value: " 1234 "})
	got, err := a.Input(ui.Field{Title: "code", Validate: ui.Required("code")})
	if err != nil || got != "1234" {
		t.Fatalf("Input = %q, %v; want 1234", got, err)
	}
	if len(*prompts) != 2 || (*prompts)[1].Error != "enter the code" {
		t.Fatalf("prompts %+v, want a second one with the error", *prompts)
	}
}

func TestPromptsCancelled(t *testing.T) {
	a, _ := testApp(t, answer{cancelled: true}, answer{cancelled: true}, answer{cancelled: true})
	if _, err := a.Input(ui.Field{Title: "x"}); !errors.Is(err, errCancelled) {
		t.Errorf("Input err = %v", err)
	}
	if _, err := a.Confirm("x", true); !errors.Is(err, errCancelled) {
		t.Errorf("Confirm err = %v", err)
	}
	if _, err := a.Choose("x", []ui.Option{{Label: "a", Value: "a"}}); !errors.Is(err, errCancelled) {
		t.Errorf("Choose err = %v", err)
	}
}

func TestChooseAndConfirm(t *testing.T) {
	a, _ := testApp(t, answer{value: "b"}, answer{value: "zzz"}, answer{value: "yes"}, answer{value: "no"})
	opts := []ui.Option{{Label: "A", Value: "a"}, {Label: "B", Value: "b"}}
	if v, err := a.Choose("pick", opts); err != nil || v != "b" {
		t.Errorf("Choose = %q, %v", v, err)
	}
	if _, err := a.Choose("pick", opts); err == nil {
		t.Error("Choose accepted a value that is not an option")
	}
	if v, _ := a.Confirm("ok?", false); !v {
		t.Error("Confirm yes = false")
	}
	if v, _ := a.Confirm("ok?", true); v {
		t.Error("Confirm no = true")
	}
}
