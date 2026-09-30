package plugin

import (
	"slices"
	"strings"
)

// preApproved is the SHA-256 (hex, comma separated) of the plugins built with this exe from this
// repo's plugins folder, set at build time by the build scripts:
//
//	-ldflags "-X aex/internal/plugin.preApproved=<hash>,<hash>"
//
// A plugin file with one of these hashes is safe without being approved.
var preApproved string

// PreApproved reports whether hash is the hash of a plugin built with this exe.
func PreApproved(hash string) bool {
	return hash != "" && slices.Contains(strings.Split(strings.ToLower(preApproved), ","), strings.ToLower(hash))
}

// pluginState is state for a plugin: pre-approved ones are safe.
func pluginState(path, hash string) (State, error) {
	if PreApproved(hash) {
		return Safe, nil
	}
	return state(path, hash)
}
