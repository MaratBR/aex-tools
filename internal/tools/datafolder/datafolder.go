// Package datafolder is the data-folder tool: opens the data folder in the file manager.
package datafolder

import (
	"flag"
	"fmt"

	"aex/internal/settings"
	"aex/internal/tool"
)

// Tool is the data-folder tool.
var Tool = tool.Tool{Name: "data-folder", Summary: "Open the data folder: settings, AEXT session, CSV exports", Run: run}

func run(args []string) error {
	usage := fmt.Sprintf(`Usage: data-folder [--print]

Opens the data folder in the file manager:
  %s
It holds .env.config, session.txt and output/. Change it with --data-dir <dir> or AEX_DATA_DIR.

  --print  Only print the path`, settings.DataDir)
	fs := flag.NewFlagSet("data-folder", flag.ContinueOnError)
	printOnly := fs.Bool("print", false, "")
	if done, err := tool.ParseFlags(fs, args, usage); done || err != nil {
		return err
	}
	fmt.Println(settings.DataDir)
	if *printOnly {
		return nil
	}
	return tool.OpenFolder(settings.DataDir)
}
