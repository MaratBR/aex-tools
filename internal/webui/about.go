package webui

import (
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
