package webui

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"aex/internal/settings"
)

// Onboarding is a screen of the window (frontend/onboarding.js) that asks to log in to each service
// the header shows a login for, opened on first start and by the debug tool onboarding.

// NeedsOnboarding reports whether the window opens the onboarding screen: it was never done or
// skipped with this data folder.
func (a *App) NeedsOnboarding() bool {
	_, err := os.Stat(settings.OnboardedFile)
	return errors.Is(err, fs.ErrNotExist)
}

// Onboarded records that the onboarding screen was done or skipped.
func (a *App) Onboarded() error {
	if err := os.MkdirAll(filepath.Dir(settings.OnboardedFile), 0o700); err != nil {
		return err
	}
	return os.WriteFile(settings.OnboardedFile, nil, 0o600)
}
