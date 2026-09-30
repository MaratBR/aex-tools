package webui

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// Marks are OSC sequences in the output, "\x1b]aex-<name>;<payload>\x07": the reader takes them
// out (they would show as nothing even if they leaked). syncMark is written by stream.Sync, the
// reader reports it reached it; any other goes to stream.mark (e.g. reminder.Mark).
const (
	markPrefix = "\x1b]aex-"
	syncMark   = markPrefix + "sync;"
	// maxMark is the longest mark taken; a longer one is passed on as text.
	maxMark = 64 << 10
)

// stream forwards what is written to w (the process's stdout and stderr) to emit, in order and
// whole UTF-8 characters at a time. Sync waits until everything written so far is forwarded, so
// a prompt sent after it never overtakes the output printed before it.
type stream struct {
	w    io.Writer
	emit func(text string)
	mark func(name, payload string) // marks other than sync, when set

	mu      sync.Mutex
	next    int
	reached map[int]chan struct{}
}

func newStream(w io.Writer, emit func(string)) *stream {
	return &stream{w: w, emit: emit, reached: map[int]chan struct{}{}}
}

// captureOutput points os.Stdout and os.Stderr (so every fmt.Print, and plugins, which inherit
// them) at a pipe streamed to emit; marks in it (other than sync) go to mark.
func captureOutput(emit func(string), mark func(name, payload string)) (*stream, error) {
	r, w, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	os.Stdout, os.Stderr = w, w
	s := newStream(w, emit)
	s.mark = mark
	go s.pump(r)
	return s, nil
}

// Sync returns once the reader has forwarded everything written before the call (or after a
// second, should the reader be stuck).
func (s *stream) Sync() {
	s.mu.Lock()
	s.next++
	id := s.next
	ch := make(chan struct{})
	s.reached[id] = ch
	s.mu.Unlock()
	fmt.Fprintf(s.w, "%s%d\x07", syncMark, id)
	select {
	case <-ch:
	case <-time.After(time.Second):
	}
	s.mu.Lock()
	delete(s.reached, id)
	s.mu.Unlock()
}

func (s *stream) pump(r io.Reader) {
	buf := make([]byte, 32*1024)
	var pending []byte
	for {
		n, err := r.Read(buf)
		pending = s.forward(append(pending, buf[:n]...))
		if err != nil {
			if !errors.Is(err, io.EOF) {
				s.emit(fmt.Sprintf("\n(output stopped: %v)\n", err))
			}
			return
		}
	}
}

// forward emits data up to what may be the start of a cut-off mark or UTF-8 character,
// handling the marks in it, and returns the rest.
func (s *stream) forward(data []byte) []byte {
	for {
		i := bytes.Index(data, []byte(markPrefix))
		if i < 0 {
			break
		}
		end := bytes.IndexByte(data[i:], '\x07')
		if end < 0 && len(data)-i <= maxMark {
			s.send(data[:i])
			return append([]byte(nil), data[i:]...)
		}
		if end < 0 || end > maxMark {
			// Not a mark of ours after all: text.
			s.send(data[:i+len(markPrefix)])
			data = data[i+len(markPrefix):]
			continue
		}
		s.send(data[:i])
		name, payload, _ := strings.Cut(string(data[i+len(markPrefix):i+end]), ";")
		if name == "sync" {
			if id, err := strconv.Atoi(payload); err == nil {
				s.mu.Lock()
				if ch := s.reached[id]; ch != nil {
					close(ch)
					delete(s.reached, id)
				}
				s.mu.Unlock()
			}
		} else if s.mark != nil {
			s.mark(name, payload)
		}
		data = data[i+end+1:]
	}
	cut := len(data)
	// A sync mark cut off at the end.
	if j := bytes.LastIndexByte(data, '\x1b'); j >= 0 && bytes.HasPrefix([]byte(syncMark), data[j:]) {
		cut = j
	}
	// A UTF-8 character cut off at the end.
	for i := 1; i <= utf8.UTFMax && i <= cut; i++ {
		if utf8.RuneStart(data[cut-i]) {
			if !utf8.FullRune(data[cut-i : cut]) {
				cut -= i
			}
			break
		}
	}
	s.send(data[:cut])
	return append([]byte(nil), data[cut:]...)
}

func (s *stream) send(b []byte) {
	if len(b) > 0 {
		s.emit(string(b))
	}
}
