// Package remind holds the reminder debug tool: it shows a reminder (internal/reminder) as a tool,
// a plugin or a widget would.
package remind

import (
	"flag"
	"fmt"
	"strings"
	"time"

	"aex/internal/reminder"
	"aex/internal/tool"
)

// Tool is the reminder tool.
var Tool = tool.Tool{Name: "reminder", Summary: "Show a reminder, as a tool or widget would", Run: run, Debug: true}

const usage = `Usage: aex debug reminder [--title <title>] [--urgent] [--in <duration>] [<message>...]

Shows a reminder on top of every window on every screen, with a chime, until it is dismissed.
  --title <title>   its title (default: Reminder)
  --urgent          extra urgent: chimes twice, and twice again every 30 s until closed or
                    10 minutes pass; marked red
  --in <duration>   waits this long first, e.g. 10s, to switch to another window (default: 0)
  <message>         what it says (default: a test message with links); links are
                    [label](link) or bare, http(s)://… or aex+<browser>://… (e.g.
                    aex+brave://google.com opens in Brave, else the default browser);
                    a ! before the link ([label](!link), !link) also closes the reminder
In a terminal it waits until the reminder is dismissed; in the window it returns at once.`

func run(args []string) error {
	fs := flag.NewFlagSet("reminder", flag.ContinueOnError)
	title := fs.String("title", "", "")
	urgent := fs.Bool("urgent", false, "")
	in := fs.Duration("in", 0, "")
	if done, err := tool.ParseFlagsAndArgs(fs, args, usage); done || err != nil {
		return err
	}
	message := strings.Join(fs.Args(), " ")
	if message == "" {
		message = "This is a test reminder from aex. Links open in a browser: [Google in Brave](aex+brave://google.com), " +
			"[DuckDuckGo in Firefox](aex+firefox://duckduckgo.com) or https://example.com in the default one; " +
			"[this one closes it too](!https://example.com). Close it on any screen to close it on all of them."
	}
	if *in > 0 {
		fmt.Printf("Showing the reminder in %s…\n", *in)
		time.Sleep(*in)
	}
	if !reminder.InWindow() {
		fmt.Println("Showing the reminder until it is dismissed…")
	}
	if err := reminder.Show(reminder.Reminder{Title: *title, Message: message, Urgent: *urgent}); err != nil {
		return err
	}
	fmt.Println("Reminder shown.")
	return nil
}
