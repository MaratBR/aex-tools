package plugin

import (
	"os"

	"aex/internal/settings"
)

// Custom tools (internal/custom), scripts run by a tool adapter, are approved like plugins: the
// SHA-256 of the file's contents is saved as its safe hash under its path, and forget-all forgets
// them too.

// Locked is a file held open with the SHA-256 of its contents. On Windows it cannot be changed,
// renamed or deleted until Close, so what is read or run meanwhile is what was hashed.
type Locked struct{ o *openFile }

// Lock opens the file at path and hashes it.
func Lock(path string) (*Locked, error) {
	o, err := openPlugin(path)
	if err != nil {
		return nil, err
	}
	return &Locked{o}, nil
}

func (l *Locked) Hash() string                     { return l.o.hash }
func (l *Locked) Stat() (os.FileInfo, error)       { return l.o.f.Stat() }
func (l *Locked) Close()                           { l.o.close() }
func (l *Locked) State(path string) (State, error) { return state(path, l.o.hash) }

// Approve asks to approve the locked file at path, the custom tool name, unless it is safe: shows
// its path, size, hash and details (label and value). On yes its hash is saved as the safe hash.
func (l *Locked) Approve(name, path string, details [][2]string) error {
	s, err := l.State(path)
	if err != nil || s == Safe {
		return err
	}
	return askApproval("Custom tool", name, path, l.o, s, details)
}

// Trust saves the locked file's hash as the safe hash of the file at path, without asking: for
// when it was already shown and approved.
func (l *Locked) Trust(path string) error {
	if err := settings.Credentials.Set(hashKey(path), l.o.hash); err != nil {
		return err
	}
	return setApproved(path, true)
}

// Forget forgets the safe hash of the file at path.
func Forget(path string) error { return forget(path) }
