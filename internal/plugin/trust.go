package plugin

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
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
func hashKey(path string) string { return "plugin-safe-sha256:" + normPath(path) }

func normPath(path string) string {
	path = filepath.Clean(path)
	if runtime.GOOS == "windows" {
		path = strings.ToLower(path)
	}
	return path
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

// forget forgets the plugin's safe hash and granted access.
func forget(path string) error {
	return errors.Join(settings.Credentials.Delete(hashKey(path)), settings.Credentials.Delete(grantKey(path)),
		setApproved(path, false))
}

// approvedKey is the credential store key listing (one per line) the paths of plugins with a safe
// hash or granted access: the store cannot list its keys, and ForgetAll has to find them.
const approvedKey = "plugin-approved-paths"

func approvedPaths() ([]string, error) {
	v, err := settings.Credentials.Get(approvedKey)
	if err != nil || v == "" {
		return nil, err
	}
	return strings.Split(v, "\n"), nil
}

// setApproved adds (on) or removes the plugin at path from the approvedKey list.
func setApproved(path string, on bool) error {
	paths, err := approvedPaths()
	if err != nil {
		return err
	}
	path = normPath(path)
	has := slices.Contains(paths, path)
	switch {
	case on && !has:
		paths = append(paths, path)
	case !on && has:
		paths = slices.DeleteFunc(paths, func(p string) bool { return p == path })
	default:
		return nil
	}
	if len(paths) == 0 {
		return settings.Credentials.Delete(approvedKey)
	}
	return settings.Credentials.Set(approvedKey, strings.Join(paths, "\n"))
}

// Approved lists the paths ForgetAll forgets: every plugin approved (or granted access) since the
// list was kept, plus the plugins in the plugins folder now.
func Approved() ([]string, error) {
	paths, err := approvedPaths()
	if err != nil {
		return nil, err
	}
	plugins, err := List()
	if err != nil {
		return nil, err
	}
	for _, p := range plugins {
		if path := normPath(p.Path); !slices.Contains(paths, path) {
			paths = append(paths, path)
		}
	}
	slices.Sort(paths)
	return paths, nil
}

// ForgetAll forgets the safe hash and granted access of every plugin (see Approved): each asks to
// be approved again on its next run. Plugin files are left alone.
func ForgetAll() error {
	paths, err := Approved()
	if err != nil {
		return err
	}
	var errs []error
	for _, path := range paths {
		errs = append(errs, settings.Credentials.Delete(hashKey(path)), settings.Credentials.Delete(grantKey(path)))
	}
	errs = append(errs, settings.Credentials.Delete(approvedKey))
	return errors.Join(errs...)
}

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
	if err := settings.Credentials.Set(hashKey(path), o.hash); err != nil {
		return err
	}
	return setApproved(path, true)
}
