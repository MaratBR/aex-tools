package plugin

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
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

// PreApprovedPlugin is a hash built in as pre-approved, and the file in the plugins folder that
// has it now ("" when none has).
type PreApprovedPlugin struct {
	Hash string `json:"hash"`
	File string `json:"file,omitempty"`
}

// PreApprovedList lists the hashes built in as pre-approved, each with the plugin file that has it.
// Only the files are read (hashed), nothing is run.
func PreApprovedList() []PreApprovedPlugin {
	var list []PreApprovedPlugin
	for h := range strings.SplitSeq(strings.ToLower(preApproved), ",") {
		if h != "" {
			list = append(list, PreApprovedPlugin{Hash: h})
		}
	}
	if len(list) == 0 {
		return nil
	}
	dir, err := Dir()
	if err != nil {
		return list
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if _, ok := pluginName(e); !ok {
			continue
		}
		hash, err := fileHash(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		if i := slices.IndexFunc(list, func(p PreApprovedPlugin) bool { return p.Hash == hash }); i >= 0 && list[i].File == "" {
			list[i].File = e.Name()
		}
	}
	return list
}

func fileHash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
