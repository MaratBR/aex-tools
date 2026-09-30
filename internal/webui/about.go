package webui

import (
	"context"
	"time"

	"aex/internal/about"
	"aex/internal/plugin"
	"aex/internal/settings"
)

// AboutInfo is what the Settings page's About section shows.
type AboutInfo struct {
	Build    about.Build    `json:"build"`
	Dev      bool           `json:"dev"` // a dev build (knows its source checkout)
	Licenses about.Licenses `json:"licenses"`
	// PreApproved are the plugin hashes built in as pre-approved, with the file that has each now.
	PreApproved []plugin.PreApprovedPlugin `json:"preApproved"`
}

// About gives the version, build, pre-approved plugins and licenses.
func (a *App) About() (AboutInfo, error) {
	l, err := about.Read()
	if err != nil {
		return AboutInfo{}, err
	}
	return AboutInfo{Build: about.Info(), Dev: settings.IsDev, Licenses: l, PreApproved: plugin.PreApprovedList()}, nil
}

// Behind gives how many commits this build is behind aex's master on GitHub, kept in the data
// folder so GitHub is asked at most every few hours (force: Check now, at most every minute).
func (a *App) Behind(force bool) about.Behind {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return about.CachedBehind(ctx, settings.DataDir, force)
}
