package webui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"aex/internal/ui"
)

// Browse opens a file or folder dialog for the open Input prompt id, as its Hint says, starting
// where current (the answer so far) points. "" when the dialog is closed without a pick.
func (a *App) Browse(id int, current string) (string, error) {
	a.mu.Lock()
	f, ok := a.fields[id]
	a.mu.Unlock()
	if !ok {
		return "", errors.New("the question is no longer open")
	}
	title := strings.TrimSuffix(f.Title, ":")
	opts := runtime.OpenDialogOptions{Title: title, DefaultDirectory: startDir(current)}
	if f.Hint.Kind == ui.HintFolder {
		return runtime.OpenDirectoryDialog(a.ctx, opts)
	}
	for _, ff := range f.Hint.Filters {
		pattern := strings.Join(ff.Patterns, ";")
		opts.Filters = append(opts.Filters, runtime.FileFilter{DisplayName: fmt.Sprintf("%s (%s)", ff.Name, pattern), Pattern: pattern})
	}
	if len(opts.Filters) > 0 {
		opts.Filters = append(opts.Filters, runtime.FileFilter{DisplayName: "All files (*.*)", Pattern: "*.*"})
	}
	return runtime.OpenFileDialog(a.ctx, opts)
}

// chooseFolderAPI opens a folder dialog for a widget (args.title), starting in args.current:
// the folder picked, "" when closed without a pick.
func chooseFolderAPI(a *App, args map[string]any) (any, error) {
	title, _ := args["title"].(string)
	current, _ := args["current"].(string)
	if title == "" {
		title = "Choose a folder"
	}
	return runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{Title: title, DefaultDirectory: startDir(current)})
}

// startDir is the folder a dialog opens in for the answer so far: that folder, or the one holding
// that file; "" (the dialog's default) when it names neither.
func startDir(current string) string {
	current = strings.Trim(strings.TrimSpace(current), `"'`)
	if current == "" {
		return ""
	}
	for _, dir := range []string{current, filepath.Dir(current)} {
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			return dir
		}
	}
	return ""
}
