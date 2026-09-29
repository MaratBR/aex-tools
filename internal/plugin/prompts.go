package plugin

// Prompts of a plugin run from the window. The plugin has no console there (its stdin is the null
// device), so its questions go to aex, which asks them in the window: aex listens on a loopback
// port for the one plugin it runs (promptServer) and passes the address and a token in the
// environment; the plugin connects (Main) and its ui.Remote sends each question there
// (promptClient).
//
// The connection carries JSON lines. The plugin sends a request; until aex answers it with a
// result, aex may send calls for the question's Field funcs (validate, paste, describe), each
// answered by the plugin with a reply.

import (
	"bufio"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"

	"aex/internal/ui"
)

const (
	promptsVar     = "AEX_PROMPTS"
	promptTokenVar = "AEX_PROMPT_TOKEN"
)

type promptRequest struct {
	Kind        string      `json:"kind"` // input | confirm | choose | tool | key | clear
	Title       string      `json:"title,omitempty"`
	Description string      `json:"description,omitempty"`
	Placeholder string      `json:"placeholder,omitempty"`
	Secret      bool        `json:"secret,omitempty"`
	Validate    bool        `json:"validate,omitempty"` // the Field funcs the plugin has
	Paste       bool        `json:"paste,omitempty"`
	Describe    bool        `json:"describe,omitempty"`
	DefaultYes  bool        `json:"defaultYes,omitempty"`
	Options     []ui.Option `json:"options,omitempty"`
}

// promptMessage is what aex sends: a call to a Field func (Call set) or the request's result.
type promptMessage struct {
	Call  string `json:"call,omitempty"` // validate | paste | describe
	Value string `json:"value,omitempty"`
	Yes   bool   `json:"yes,omitempty"`
	Error string `json:"error,omitempty"`
}

// promptReply answers a call: the text paste or describe gives, or the error validate gives.
type promptReply struct {
	Text  string `json:"text,omitempty"`
	Error string `json:"error,omitempty"`
}

// Host side -----------------------------------------------------------------------------------

// promptServer asks the questions of the plugin that connects with its token, through ui.
type promptServer struct {
	ln    net.Listener
	token string

	mu   sync.Mutex
	conn net.Conn
}

// servePrompts starts a promptServer; env is what the plugin needs to reach it.
func servePrompts() (s *promptServer, env []string, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return nil, nil, err
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, nil, err
	}
	s = &promptServer{ln: ln, token: hex.EncodeToString(b)}
	go s.accept()
	return s, []string{promptsVar + "=" + ln.Addr().String(), promptTokenVar + "=" + s.token}, nil
}

// Close stops listening and drops the plugin's connection.
func (s *promptServer) Close() {
	s.ln.Close()
	s.mu.Lock()
	if s.conn != nil {
		s.conn.Close()
	}
	s.mu.Unlock()
}

// accept takes the first connection that sends the token, then serves it alone.
func (s *promptServer) accept() {
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			return
		}
		r := bufio.NewReader(conn)
		line, err := r.ReadString('\n')
		if err != nil || subtle.ConstantTimeCompare([]byte(strings.TrimSpace(line)), []byte(s.token)) != 1 {
			conn.Close()
			continue
		}
		s.ln.Close()
		s.mu.Lock()
		s.conn = conn
		s.mu.Unlock()
		s.serve(conn, r)
		return
	}
}

func (s *promptServer) serve(conn net.Conn, r *bufio.Reader) {
	defer conn.Close()
	dec := json.NewDecoder(r)
	enc := json.NewEncoder(conn)
	for {
		var req promptRequest
		if err := dec.Decode(&req); err != nil {
			return
		}
		// Calls are made, and the result sent, under mu while the request is open; once it is
		// closed, a late call (describe runs off the question's goroutine) gives nothing.
		var mu sync.Mutex
		open := true
		call := func(name, value string) (promptReply, bool) {
			mu.Lock()
			defer mu.Unlock()
			var rep promptReply
			if !open || enc.Encode(promptMessage{Call: name, Value: value}) != nil || dec.Decode(&rep) != nil {
				return promptReply{}, false
			}
			return rep, true
		}
		msg := s.ask(req, call)
		mu.Lock()
		open = false
		err := enc.Encode(msg)
		mu.Unlock()
		if err != nil {
			return
		}
	}
}

// ask asks req through ui and gives its result; call calls the plugin's Field funcs.
func (s *promptServer) ask(req promptRequest, call func(name, value string) (promptReply, bool)) promptMessage {
	var msg promptMessage
	var err error
	switch req.Kind {
	case "input":
		f := ui.Field{Title: req.Title, Description: req.Description, Placeholder: req.Placeholder, Secret: req.Secret}
		if req.Validate {
			f.Validate = func(v string) error {
				rep, ok := call("validate", v)
				if !ok {
					return errors.New("the plugin stopped")
				}
				if rep.Error != "" {
					return errors.New(rep.Error)
				}
				return nil
			}
		}
		if req.Paste {
			f.Paste = func(v string) string {
				if rep, ok := call("paste", v); ok {
					return rep.Text
				}
				return v
			}
		}
		if req.Describe {
			f.Describe = func(v string) string { rep, _ := call("describe", v); return rep.Text }
		}
		msg.Value, err = ui.Input(f)
	case "confirm":
		msg.Yes, err = ui.Confirm(req.Title, req.DefaultYes)
	case "choose":
		msg.Value, err = ui.Choose(req.Title, req.Options)
	case "tool":
		msg.Value, err = ui.PickTool(req.Title, req.Options)
	case "key":
		ui.WaitKey()
	case "clear":
		ui.ClearScreen()
	default:
		err = fmt.Errorf("unknown question kind %q", req.Kind)
	}
	if err != nil {
		msg.Error = err.Error()
	}
	return msg
}

// Plugin side ---------------------------------------------------------------------------------

// connectPrompts points ui.Remote at aex when aex passed where to ask questions, taking that out of
// the environment so processes the plugin starts do not get it.
func connectPrompts() error {
	addr, token := os.Getenv(promptsVar), os.Getenv(promptTokenVar)
	os.Unsetenv(promptsVar)
	os.Unsetenv(promptTokenVar)
	if addr == "" {
		return nil
	}
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return fmt.Errorf("connecting to aex for questions: %w", err)
	}
	if _, err := fmt.Fprintln(conn, token); err != nil {
		conn.Close()
		return fmt.Errorf("connecting to aex for questions: %w", err)
	}
	ui.Remote = &promptClient{enc: json.NewEncoder(conn), dec: json.NewDecoder(bufio.NewReader(conn))}
	return nil
}

// promptClient is a plugin's ui.Remote: it sends each question to aex.
type promptClient struct {
	mu  sync.Mutex
	enc *json.Encoder
	dec *json.Decoder
}

// ask sends req and answers the calls for f (nil for none) until the result comes.
func (c *promptClient) ask(req promptRequest, f *ui.Field) (promptMessage, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.enc.Encode(req); err != nil {
		return promptMessage{}, fmt.Errorf("asking aex: %w", err)
	}
	for {
		var msg promptMessage
		if err := c.dec.Decode(&msg); err != nil {
			return promptMessage{}, fmt.Errorf("asking aex: %w", err)
		}
		if msg.Call == "" {
			if msg.Error != "" {
				return msg, errors.New(msg.Error)
			}
			return msg, nil
		}
		var rep promptReply
		switch {
		case f == nil:
		case msg.Call == "validate" && f.Validate != nil:
			if err := f.Validate(msg.Value); err != nil {
				rep.Error = err.Error()
			}
		case msg.Call == "paste" && f.Paste != nil:
			rep.Text = f.Paste(msg.Value)
		case msg.Call == "describe" && f.Describe != nil:
			rep.Text = f.Describe(msg.Value)
		}
		if err := c.enc.Encode(rep); err != nil {
			return promptMessage{}, fmt.Errorf("asking aex: %w", err)
		}
	}
}

func (c *promptClient) Input(f ui.Field) (string, error) {
	msg, err := c.ask(promptRequest{Kind: "input", Title: f.Title, Description: f.Description, Placeholder: f.Placeholder,
		Secret: f.Secret, Validate: f.Validate != nil, Paste: f.Paste != nil, Describe: f.Describe != nil}, &f)
	return msg.Value, err
}

func (c *promptClient) Confirm(question string, defaultYes bool) (bool, error) {
	msg, err := c.ask(promptRequest{Kind: "confirm", Title: question, DefaultYes: defaultYes}, nil)
	return msg.Yes, err
}

func (c *promptClient) Choose(title string, options []ui.Option) (string, error) {
	msg, err := c.ask(promptRequest{Kind: "choose", Title: title, Options: options}, nil)
	return msg.Value, err
}

func (c *promptClient) PickTool(title string, tools []ui.Option) (string, error) {
	msg, err := c.ask(promptRequest{Kind: "tool", Title: title, Options: tools}, nil)
	return msg.Value, err
}

func (c *promptClient) WaitKey()     { c.ask(promptRequest{Kind: "key"}, nil) }
func (c *promptClient) ClearScreen() { c.ask(promptRequest{Kind: "clear"}, nil) }
