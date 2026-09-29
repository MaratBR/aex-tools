// jira-release-handoff: plugin tool (see internal/plugin), built to plugins\jira-release-handoff.exe
// next to aex.exe. Does nothing yet.
package main

import (
	"flag"
	"fmt"

	"aex/internal/plugin"
	"aex/internal/tool"
)

func main() {
	plugin.Main(tool.Tool{Name: "jira-release-handoff", Summary: "Jira release handoff (not implemented yet)", Run: run})
}

func run(args []string) error {
	usage := `Usage: jira-release-handoff

Not implemented yet.`
	fs := flag.NewFlagSet("jira-release-handoff", flag.ContinueOnError)
	if done, err := tool.ParseFlags(fs, args, usage); done || err != nil {
		return err
	}
	fmt.Println("jira-release-handoff: nothing to do yet.")
	return nil
}
