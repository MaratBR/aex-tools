package composer

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"aex/internal/tool"
	"aex/internal/ui"
)

// Tools gives the tool list (built-in tools, plugins, custom and composed tools), for tool steps.
// Set by main.
var Tools = func() []tool.Tool { return nil }

// Run is what an action gets to run with.
type Run struct {
	// Ctx ends when the step should stop: a critical step running with it failed.
	Ctx context.Context
	// stack are the composed tools running, outermost first: a step cannot run one of them again.
	stack []string
	// prefix starts every line printed while steps run together: "[2] ".
	prefix string
	// indent is put before every line of the steps of an if or a check, under it.
	indent string
}

// Printf prints a line of the step's own, after the step's prefix when it runs with others.
func (r *Run) Printf(format string, args ...any) {
	fmt.Printf("%s%s%s\n", r.prefix, r.indent, fmt.Sprintf(format, args...))
}

// nested is r for the steps of an if or a check: their lines go under it, indented.
func (r *Run) nested() *Run {
	c := *r
	c.indent += "   "
	return &c
}

// Output is where a program the step runs prints: w, each line after the step's prefix when it
// runs with others, and indented like the step's own lines.
func (r *Run) Output(w io.Writer) io.Writer {
	if r.prefix+r.indent == "" {
		return w
	}
	return &prefixWriter{w: w, prefix: r.prefix + r.indent, start: true}
}

// prefixWriter puts prefix before every line written to w.
type prefixWriter struct {
	mu     sync.Mutex
	w      io.Writer
	prefix string
	start  bool // the next byte starts a line
}

func (p *prefixWriter) Write(b []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	var out bytes.Buffer
	for _, c := range b {
		if p.start {
			out.WriteString(p.prefix)
			p.start = false
		}
		out.WriteByte(c)
		p.start = c == '\n'
	}
	if _, err := p.w.Write(out.Bytes()); err != nil {
		return 0, err
	}
	return len(b), nil
}

// ErrInterrupted is how a composed tool ends when it is interrupted (Interrupt).
var ErrInterrupted = errors.New("interrupted")

// OnInterruptible, when set, is told when a composed tool starts that Interrupt can stop (true),
// and when it ends (false). Set by internal/webui, which shows a Stop button meanwhile.
var OnInterruptible func(bool)

var (
	interruptMu sync.Mutex
	interrupt   context.CancelCauseFunc // the composed tool running from the tool list, nil when none
)

// Interrupt stops the composed tool running: the steps running stop, those that can (a program
// waited for is killed, a delay, check or message stops; an aex tool step ends first), and no more
// run. It reports whether one was running.
func Interrupt() bool {
	interruptMu.Lock()
	defer interruptMu.Unlock()
	if interrupt == nil {
		return false
	}
	interrupt(ErrInterrupted)
	return true
}

// interruptible runs c so that Interrupt can stop it; one running in another (already
// interruptible) runs as it is.
func interruptible(c Composed) error {
	interruptMu.Lock()
	if interrupt != nil {
		interruptMu.Unlock()
		return Execute(context.Background(), c)
	}
	ctx, cancel := context.WithCancelCause(context.Background())
	interrupt = cancel
	interruptMu.Unlock()
	if OnInterruptible != nil {
		OnInterruptible(true)
	}
	defer func() {
		interruptMu.Lock()
		interrupt = nil
		interruptMu.Unlock()
		cancel(nil)
		if OnInterruptible != nil {
			OnInterruptible(false)
		}
	}()
	return Execute(ctx, c)
}

// interrupted reports whether ctx ended by Interrupt.
func interrupted(ctx context.Context) bool { return errors.Is(context.Cause(ctx), ErrInterrupted) }

// Execute runs the composed tool c: its steps in order, steps marked parallel next to one another
// together. A step that fails is reported and the next one runs, unless it is critical: then c
// stops after the steps running with it end (those that can stop are stopped). It fails when any
// step failed. A return step ends it, as it says: worked or failed.
func Execute(ctx context.Context, c Composed) error { return execute(ctx, c, nil) }

func execute(ctx context.Context, c Composed, stack []string) error {
	if i := slices.IndexFunc(stack, func(n string) bool { return strings.EqualFold(n, c.Name) }); i >= 0 {
		return fmt.Errorf("%s runs itself: %s", c.Name, strings.Join(append(slices.Clip(stack[i:]), c.Name), " → "))
	}
	if len(c.Steps) == 0 {
		return fmt.Errorf("%s has no steps", c.Name)
	}
	out := ui.Out
	started := time.Now()
	err := runSteps(&Run{Ctx: ctx, stack: append(slices.Clip(stack), c.Name)}, c.Steps)
	took := since(started)
	if interrupted(ctx) {
		if len(stack) == 0 {
			fmt.Println(out.Yellow(fmt.Sprintf("■ %s interrupted in %s", c.Name, took)))
		}
		return ErrInterrupted
	}
	if ret, ok := errors.AsType[*Returned](err); ok {
		if ret.Fail {
			fmt.Println(out.Red(fmt.Sprintf("■ %s returned failed in %s%s", c.Name, took, colonMessage(ret.Message))))
			return ret
		}
		fmt.Println(out.Green(fmt.Sprintf("■ %s returned worked in %s%s", c.Name, took, colonMessage(ret.Message))))
		return nil
	}
	if stop, ok := errors.AsType[*stopped](err); ok {
		fmt.Println(out.Red(fmt.Sprintf("■ %s stopped in %s: %s", c.Name, took, stop.Error())))
		return err
	}
	if fails, ok := errors.AsType[*failures](err); ok {
		fmt.Println(out.Yellow(fmt.Sprintf("■ %s done in %s, %s", c.Name, took, fails.Error())))
		return err
	}
	if err != nil {
		return err
	}
	fmt.Println(out.Green(fmt.Sprintf("■ %s done in %s, all %d steps worked", c.Name, took, len(c.Steps))))
	return nil
}

func colonMessage(msg string) string {
	if msg == "" {
		return ""
	}
	return ": " + msg
}

// Returned is how a return step ends the steps it is in, up to the check it is in (one try of it)
// or else the composed tool: worked, or failed (Fail), and why (Message).
type Returned struct {
	Fail    bool
	Message string
}

func (r *Returned) Error() string {
	what := "returned worked"
	if r.Fail {
		what = "returned failed"
	}
	return what + colonMessage(r.Message)
}

// stopped is steps ending at a critical step that failed.
type stopped struct {
	n     int // its number, from 1
	title string
	left  int // the steps not run
}

func (s *stopped) Error() string {
	left := ""
	if s.left > 0 {
		left = fmt.Sprintf(", %d not run", s.left)
	}
	return fmt.Sprintf("step %d (%s) is critical and failed%s", s.n, s.title, left)
}

// failures is steps that all ran, some failing.
type failures struct{ failed, total int }

func (f *failures) Error() string {
	if f.total == 1 {
		return "its step failed"
	}
	return fmt.Sprintf("%d of %d steps failed", f.failed, f.total)
}

// runSteps runs steps in order, those marked parallel next to one another together. A failed
// step is reported and the next one runs, unless it is critical: then they stop (*stopped). A
// return step ends them with its *Returned. Else they fail (*failures) when any step failed.
func runSteps(r *Run, steps []Step) error {
	total, failed := len(steps), 0
	for i := 0; i < total; {
		j := i + 1
		if steps[i].Parallel {
			for j < total && steps[j].Parallel {
				j++
			}
		}
		errs := runBatch(r, steps, i, j)
		stop := -1
		var ret *Returned
		for k, err := range errs {
			if rr, ok := errors.AsType[*Returned](err); ok {
				if ret == nil {
					ret = rr
				}
				continue
			}
			if err != nil {
				failed++
				if steps[i+k].Critical && stop < 0 {
					stop = i + k
				}
			}
		}
		if stop >= 0 {
			return &stopped{n: stop + 1, title: steps[stop].Title(), left: total - j}
		}
		if ret != nil {
			return ret
		}
		if err := r.Ctx.Err(); err != nil {
			return err
		}
		i = j
	}
	if failed > 0 {
		return &failures{failed, total}
	}
	return nil
}

// runBatch runs steps[i:j], together when there are several, and gives each one's error.
func runBatch(r *Run, steps []Step, i, j int) []error {
	errs := make([]error, j-i)
	total := len(steps)
	if j-i == 1 {
		errs[0] = runStep(r, steps[i], i+1, total)
		return errs
	}
	r.Printf("%s", ui.Out.Bold(fmt.Sprintf("▸ Steps %d–%d of %d, together", i+1, j, total)))
	batch, cancel := context.WithCancel(r.Ctx)
	defer cancel()
	var wg sync.WaitGroup
	for k := i; k < j; k++ {
		wg.Go(func() {
			sub := *r
			sub.Ctx = batch
			sub.prefix = r.prefix + ui.Out.Dim(fmt.Sprintf("[%d] ", k+1))
			err := runStep(&sub, steps[k], k+1, total)
			errs[k-i] = err
			// A critical step failing stops the others, those that can stop.
			if err != nil && steps[k].Critical {
				if _, ret := errors.AsType[*Returned](err); !ret {
					cancel()
				}
			}
		})
	}
	wg.Wait()
	return errs
}

// runStep runs step n of total, saying when it starts and how it ended. A return says what it
// returns; a step a return ended (an if around it) says nothing more.
func runStep(r *Run, s Step, n, total int) (err error) {
	out := ui.Out
	r.Printf("%s", out.Bold(fmt.Sprintf("▸ Step %d/%d · %s", n, total, s.Title())))
	started := time.Now()
	err = runAction(r, s.Action)
	if ret, ok := errors.AsType[*Returned](err); ok {
		if _, isReturn := s.Action.(*ReturnAction); isReturn {
			color := out.Green
			if ret.Fail {
				color = out.Red
			}
			r.Printf("%s", color("↩ "+capital(ret.Error())))
		}
		return err
	}
	if err != nil && interrupted(r.Ctx) {
		r.Printf("%s", out.Yellow(fmt.Sprintf("■ Step %d interrupted after %s", n, since(started))))
		return ErrInterrupted
	}
	if err == nil {
		r.Printf("%s", out.Green(fmt.Sprintf("✔ Step %d done in %s", n, since(started))))
		return nil
	}
	what := "failed"
	if s.Critical {
		what = "failed (critical)"
	}
	r.Printf("%s %s", out.Red(fmt.Sprintf("✖ Step %d %s:", n, what)), err)
	return err
}

func capital(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// runAction runs a, a panic in it being its error.
func runAction(r *Run, a Action) (err error) {
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("panic: %v", p)
		}
	}()
	if err := r.Ctx.Err(); err != nil {
		return fmt.Errorf("not run: %w", err)
	}
	return a.Run(r)
}

func since(t time.Time) string {
	d := time.Since(t)
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	return d.Round(time.Second).String()
}

// sleep waits d, or until ctx ends (then false).
func sleep(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return true
	case <-ctx.Done():
		return false
	}
}

// Usage is a composed tool's --help: what it does and its steps.
func Usage(c Composed) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Usage: %s %s\n\n", Group, c.Name)
	if c.Summary != "" {
		fmt.Fprintf(&b, "%s\n\n", c.Summary)
	}
	b.WriteString("A composed tool: runs its steps in order. A failed step is reported and the next one runs,\nunless it is critical: then it stops. Steps marked parallel next to one another run together.\n\nSteps:\n")
	usageSteps(&b, c.Steps, "  ")
	return strings.TrimRight(b.String(), "\n")
}

// usageSteps lists steps, those of an if or a check under it.
func usageSteps(b *strings.Builder, steps []Step, indent string) {
	for i, s := range steps {
		var marks []string
		if s.Critical {
			marks = append(marks, "critical")
		}
		if s.Parallel {
			marks = append(marks, "parallel")
		}
		line := fmt.Sprintf("%s%d. %s", indent, i+1, s.Title())
		if s.Label != "" && s.Action != nil {
			line += " (" + s.Action.Describe() + ")"
		}
		if len(marks) > 0 {
			line += "  [" + strings.Join(marks, ", ") + "]"
		}
		b.WriteString(line + "\n")
		switch a := s.Action.(type) {
		case *CheckAction:
			usageSteps(b, a.Do, indent+"   ")
		case *IfAction:
			b.WriteString(indent + "   then:\n")
			usageSteps(b, a.Then, indent+"     ")
			if len(a.Else) > 0 {
				b.WriteString(indent + "   else:\n")
				usageSteps(b, a.Else, indent+"     ")
			}
		}
	}
}

// Discover is the composed tools as tools, in one group (Group), none when there are none. The
// group is skipped with a warning when another tool in taken has its name, as are composed tools
// named like one. One that cannot be read is listed with why, and fails when run.
func Discover(taken []tool.Tool) []tool.Tool {
	entries, err := Load()
	if err != nil {
		ui.Warn("composed tools: %v", err)
		return nil
	}
	if len(entries) == 0 {
		return nil
	}
	isTaken := func(name string) bool {
		return slices.ContainsFunc(taken, func(t tool.Tool) bool { return strings.EqualFold(t.Name, name) })
	}
	if isTaken(Group) {
		ui.Warn("composed tools: skipped, another tool is called %s", Group)
		return nil
	}
	var subs []tool.Tool
	for _, e := range entries {
		if isTaken(e.Name) {
			ui.Warn("composed tools: skipped %s, another tool has that name", e.Name)
			continue
		}
		t := tool.Tool{Name: e.Name, Summary: e.Summary, Run: runner(e)}
		if t.Summary == "" {
			t.Summary = fmt.Sprintf("Composed tool: %d step%s", len(e.Steps), map[bool]string{true: "s"}[len(e.Steps) != 1])
		}
		if e.Err != nil {
			t.Warn = "Composed tool cannot run: " + e.Err.Error()
			t.Summary = e.Err.Error()
		}
		subs = append(subs, t)
	}
	if len(subs) == 0 {
		return nil
	}
	return []tool.Tool{{Name: Group, Summary: "Composed tools: tools made of steps, in the Composer", Sub: subs}}
}

func runner(e Entry) func(args []string) error {
	return func(args []string) error {
		if e.Err != nil {
			return fmt.Errorf("composed tool %s cannot be read: %w", e.Name, e.Err)
		}
		if len(args) == 1 && slices.Contains([]string{"--help", "-h", "-?"}, args[0]) {
			fmt.Println(Usage(e.Composed))
			return nil
		}
		if len(args) > 0 {
			return fmt.Errorf("composed tools take no arguments (see %s %s --help)", Group, e.Name)
		}
		return interruptible(e.Composed)
	}
}

// SplitArgs splits an args line as a command line: spaces split, "double quotes" keep spaces in
// (and are dropped), as in --name="a b".
func SplitArgs(line string) []string {
	var args []string
	var cur strings.Builder
	inQuotes, has := false, false
	for _, c := range line {
		switch {
		case c == '"':
			inQuotes, has = !inQuotes, true
		case !inQuotes && (c == ' ' || c == '\t' || c == '\n' || c == '\r'):
			if has {
				args = append(args, cur.String())
				cur.Reset()
				has = false
			}
		default:
			cur.WriteRune(c)
			has = true
		}
	}
	if has {
		args = append(args, cur.String())
	}
	return args
}

// expand expands a path the user typed: ~ at its start is the home folder, and environment
// variables ($NAME, ${NAME}, and on Windows %NAME%) their values. Quotes around it are dropped.
func expand(path string) string {
	path = strings.Trim(strings.TrimSpace(path), `"'`)
	if path == "~" || strings.HasPrefix(path, "~/") || strings.HasPrefix(path, `~\`) {
		if home, err := os.UserHomeDir(); err == nil {
			path = home + path[1:]
		}
	}
	path = winVars(path)
	return os.Expand(path, func(name string) string {
		if v, ok := os.LookupEnv(name); ok {
			return v
		}
		return "$" + name
	})
}
