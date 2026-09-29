package webui

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"sync"
	"time"
	"unicode/utf8"
)

// syncMark is written into the output pipe by stream.Sync; the reader drops it and reports it
// reached it. An OSC sequence, so it would show as nothing even if it leaked.
const syncMark = "\x1b]aex-sync;"

// stream forwards what is written to w (the process's stdout and stderr) to emit, in order and
// whole UTF-8 characters at a time. Sync waits until everything written so far is forwarded, so
// a prompt sent after it never overtakes the output printed before it.
type stream struct {
	w    io.Writer
	emit func(text string)

	mu      sync.Mutex
	next    int
	reached map[int]chan struct{}
}

func newStream(w io.Writer, emit func(string)) *stream {
	return &stream{w: w, emit: emit, reached: map[int]chan struct{}{}}
}

// captureOutput points os.Stdout and os.Stderr (so every fmt.Print, and plugins, which inherit
// them) at a pipe streamed to emit.
func captureOutput(emit func(string)) (*stream, error) {
	r, w, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	os.Stdout, os.Stderr = w, w
	s := newStream(w, emit)
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

// forward emits data up to what may be the start of a cut-off sync mark or UTF-8 character,
// handling the sync marks in it, and returns the rest.
func (s *stream) forward(data []byte) []byte {
	for {
		i := bytes.Index(data, []byte(syncMark))
		if i < 0 {
			break
		}
		end := bytes.IndexByte(data[i:], '\x07')
		if end < 0 {
			s.send(data[:i])
			return append([]byte(nil), data[i:]...)
		}
		s.send(data[:i])
		if id, err := strconv.Atoi(string(data[i+len(syncMark) : i+end])); err == nil {
			s.mu.Lock()
			if ch := s.reached[id]; ch != nil {
				close(ch)
				delete(s.reached, id)
			}
			s.mu.Unlock()
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
