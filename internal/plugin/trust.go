package plugin

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"aex/internal/settings"
	"aex/internal/ui"
)

// A plugin runs only after the user approved its file: the SHA-256 of the file's contents they
// approved (its safe hash) is kept in the credential store under the file's path, and every run
// (--aex-describe too) hashes the file again and checks it still matches.

// State is whether a plugin's file is approved.
type State int

const (
	Safe        State = iota // its hash is the safe hash
	NotApproved              // there is no safe hash
	Changed                  // its hash is not the safe hash
)

func (s State) String() string {
	return [...]string{"safe", "not approved yet", "changed since it was approved"}[s]
}

// hashKey is the credential store key of the safe hash of the plugin at path.
func hashKey(path string) string {
	path = filepath.Clean(path)
	if runtime.GOOS == "windows" {
		path = strings.ToLower(path)
	}
	return "plugin-safe-sha256:" + path
}

func state(path, hash string) (State, error) {
	safe, err := settings.Credentials.Get(hashKey(path))
	switch {
	case err != nil:
		return 0, err
	case safe == "":
		return NotApproved, nil
	case safe != hash:
		return Changed, nil
	}
	return Safe, nil
}

func forget(path string) error { return settings.Credentials.Delete(hashKey(path)) }

// openFile is a plugin file held open, with its SHA-256. On Windows it cannot be changed, renamed
// or deleted while open, so what runs is what was hashed.
type openFile struct {
	f    *os.File
	hash string
}

func openPlugin(path string) (*openFile, error) {
	f, err := openLocked(path)
	if err != nil {
		return nil, err
	}
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		f.Close()
		return nil, err
	}
	return &openFile{f, hex.EncodeToString(h.Sum(nil))}, nil
}

func (o *openFile) close() { o.f.Close() }

// start starts cmd (for this file) and then lets go of the file: once started, the image is loaded.
func (o *openFile) start(cmd *exec.Cmd) error {
	defer o.close()
	return cmd.Start()
}

// approve opens the plugin at path, first asking the user to approve it if it is not safe. On yes
// its hash is saved as the safe hash.
func approve(name, path string) (*openFile, error) {
	o, err := openPlugin(path)
	if err != nil {
		return nil, err
	}
	s, err := state(path, o.hash)
	if err == nil && s != Safe {
		err = askApproval(name, path, o, s)
	}
	if err != nil {
		o.close()
		return nil, err
	}
	return o, nil
}

func askApproval(name, path string, o *openFile, s State) error {
	if err := ui.AssertInteractive("approving plugin " + name); err != nil {
		return fmt.Errorf("plugin %s is %v: %w", name, s, err)
	}
	c := ui.Err
	fmt.Fprintf(os.Stderr, "%s Plugin %s is %v. It would run with your rights.\n", c.Bold(c.Yellow("▲")), c.Bold(name), s)
	fmt.Fprintf(os.Stderr, "  %s %s\n", c.Dim("File     "), path)
	if stat, err := o.f.Stat(); err == nil {
		fmt.Fprintf(os.Stderr, "  %s %s\n", c.Dim("Size     "), sizeAndTime(stat.Size(), stat.ModTime()))
	}
	fmt.Fprintf(os.Stderr, "  %s %s\n", c.Dim("SHA-256  "), c.Bold(o.hash))
	if s == Changed {
		if safe, err := settings.Credentials.Get(hashKey(path)); err == nil {
			fmt.Fprintf(os.Stderr, "  %s %s\n", c.Dim("Safe hash"), safe)
		}
	}
	yes, err := ui.Confirm("Save this hash as safe and run the file?", false)
	if err != nil {
		return err
	}
	if !yes {
		return fmt.Errorf("plugin %s not approved", name)
	}
	return settings.Credentials.Set(hashKey(path), o.hash)
}
