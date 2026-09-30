// Package about is what the window's About section shows: aex's version, build and license, and
// the licenses of everything built into it (about.json, written by ./gen from the source).
package about

import (
	_ "embed"
	"encoding/json"
	"os"
	"runtime"
	"runtime/debug"
	"time"
)

//go:generate go run ./gen

// Repo is aex's source repository.
const Repo = "https://github.com/MaratBR/aex-tools"

//go:embed about.json
var aboutJSON []byte

// builtAt is when the exe was built (RFC 3339), set by the release build:
//
//	-ldflags "-X aex/internal/about.builtAt=2026-09-30T10:00:00Z"
var builtAt string

// File is a license file of a component, by its path in it.
type File struct {
	Path string `json:"path"`
	Text string `json:"text"`
}

// Component is something built into aex under a license of its own.
type Component struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
	URL     string `json:"url"`
	License string `json:"license"` // SPDX expression
	Note    string `json:"note,omitempty"`
	Files   []File `json:"files,omitempty"`
}

// Licenses are aex's license (SPDX id and text), its NOTICE and the third-party components.
type Licenses struct {
	Version    string      `json:"version"`
	License    string      `json:"license"`
	Text       string      `json:"text"`
	Notice     string      `json:"notice"`
	ThirdParty []Component `json:"thirdParty"`
}

// Build is this exe's version and how it was built.
type Build struct {
	Version string `json:"version"`
	// BuiltAt is when it was built; for a build that did not set it, the exe file's modified time
	// (BuiltAtFile).
	BuiltAt     time.Time `json:"builtAt,omitzero"`
	BuiltAtFile bool      `json:"builtAtFile,omitempty"`
	// Commit is the git commit it was built from (none for go run); Modified says the checkout had
	// changes not committed.
	Commit     string    `json:"commit,omitempty"`
	CommitURL  string    `json:"commitURL,omitempty"`
	CommitTime time.Time `json:"commitTime,omitzero"`
	Modified   bool      `json:"modified,omitempty"`
	Repo       string    `json:"repo"`
	Go         string    `json:"go"`
	Platform   string    `json:"platform"`
}

// Read gives the licenses built in.
func Read() (Licenses, error) {
	var l Licenses
	if err := json.Unmarshal(aboutJSON, &l); err != nil {
		return l, err
	}
	for i := range l.ThirdParty {
		if l.ThirdParty[i].Version == "" && l.ThirdParty[i].Name == "Go standard library" {
			l.ThirdParty[i].Version = runtime.Version()
		}
	}
	return l, nil
}

// Info gives this exe's build.
func Info() Build {
	b := Build{Repo: Repo, Go: runtime.Version(), Platform: runtime.GOOS + "/" + runtime.GOARCH}
	if l, err := Read(); err == nil {
		b.Version = l.Version
	}
	if t, err := time.Parse(time.RFC3339, builtAt); err == nil {
		b.BuiltAt = t
	} else if exe, err := os.Executable(); err == nil {
		if st, err := os.Stat(exe); err == nil {
			b.BuiltAt, b.BuiltAtFile = st.ModTime(), true
		}
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, s := range info.Settings {
			switch s.Key {
			case "vcs.revision":
				b.Commit = s.Value
				b.CommitURL = Repo + "/commit/" + s.Value
			case "vcs.time":
				b.CommitTime, _ = time.Parse(time.RFC3339, s.Value)
			case "vcs.modified":
				b.Modified = s.Value == "true"
			}
		}
	}
	return b
}
