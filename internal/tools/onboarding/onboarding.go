// Package onboarding is the onboarding debug tool: it opens the window's onboarding screen (logging
// in to each service), which otherwise shows on first start only.
package onboarding

import (
	"errors"

	"aex/internal/tool"
)

// Tool is the onboarding tool. The window opens its onboarding screen for it instead of running it
// (app.js: windowCommands).
var Tool = tool.Tool{Name: "onboarding", Summary: "Open the onboarding screen, as on first start", Run: run, Debug: true}

func run([]string) error {
	return errors.New("the onboarding screen is in the window: open it with aex --debug, then run debug onboarding there")
}
