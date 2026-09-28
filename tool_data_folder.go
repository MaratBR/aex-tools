package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"runtime"

	"aex/internal/settings"
)

func dataFolder(args []string) error {
	usage := fmt.Sprintf(`Usage: data-folder [--print]

Opens the data folder in the file manager:
  %s
It holds .env.config, session.txt and output/. Change it with --data-dir <dir> or AEX_DATA_DIR.

  --print  Only print the path`, settings.DataDir)
	fs := flag.NewFlagSet("data-folder", flag.ContinueOnError)
	printOnly := fs.Bool("print", false, "")
	if done, err := parseFlags(fs, args, usage); done || err != nil {
		return err
	}
	fmt.Println(settings.DataDir)
	if *printOnly {
		return nil
	}
	if err := os.MkdirAll(settings.DataDir, 0o755); err != nil {
		return err
	}
	opener := "xdg-open"
	switch runtime.GOOS {
	case "windows":
		opener = "explorer.exe"
	case "darwin":
		opener = "open"
	}
	// Not waited for: the file manager outlives this process (explorer.exe also exits non-zero on success).
	if cmd := exec.Command(opener, settings.DataDir); cmd.Start() == nil {
		cmd.Process.Release()
	}
	return nil
}
