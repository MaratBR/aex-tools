// Package onboarding holds the onboarding debug tools: onboarding opens the window's onboarding
// screen (logging in to each service), which otherwise shows on first start only, and
// reset-onboarding makes it show again on the next start.
package onboarding

import (
	"errors"
	"fmt"
	"io/fs"
	"os"

	"aex/internal/settings"
	"aex/internal/tool"
)

// Tool is the onboarding tool. The window opens its onboarding screen for it instead of running it
// (app.js: windowCommands).
var Tool = tool.Tool{Name: "onboarding", Summary: "Open the onboarding screen, as on first start", Run: run, Debug: true}

// ResetTool is the reset-onboarding tool: it removes settings.OnboardedFile, so the window opens
// its onboarding screen on the next start.
var ResetTool = tool.Tool{Name: "reset-onboarding", Summary: "Show the onboarding screen again on the next start", Run: reset, Debug: true}

func run([]string) error {
	return errors.New("the onboarding screen is in the window: open it with aex --debug, then run debug onboarding there")
}

func reset([]string) error {
	if err := os.Remove(settings.OnboardedFile); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	fmt.Println("Onboarding reset: the window opens it on the next start.")
	return nil
}
