// Package settings knows where things live (app root, data folder) and loads settings from
// .env files, the environment and app settings (.env.config).
package settings

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

var (
	// IsDev is true for a build that knows its source checkout (go build without -trimpath, as
	// bin\*.ps1 do); release builds (bin\build-exe.ps1) use -trimpath.
	IsDev bool
	// AppRoot holds .env and .env.private: the repo for a dev build, else the folder holding the exe.
	AppRoot string
	// DataDir holds app settings (.env.config), the AEXT session and output files.
	DataDir        string
	JiraExportDir  string
	SharedEnvFile  string
	PrivateEnvFile string
	// ConfigEnvFile holds app settings: auth settings saved by configure and the prompts, and the
	// AppSettings defaults written on first start.
	ConfigEnvFile string
)

// TakeGlobalArgs removes the option every entry point accepts, --data-dir <dir> (or
// --data-dir=<dir>), and returns its value (last one wins) and the other args.
func TakeGlobalArgs(argv []string) (dataDir string, rest []string, err error) {
	for i := 0; i < len(argv); i++ {
		arg := argv[i]
		switch {
		case arg == "--data-dir":
			i++
			if i >= len(argv) || argv[i] == "" {
				return "", nil, errors.New("--data-dir needs a folder")
			}
			dataDir = argv[i]
		case strings.HasPrefix(arg, "--data-dir="):
			dataDir = strings.TrimPrefix(arg, "--data-dir=")
		default:
			rest = append(rest, arg)
		}
	}
	return dataDir, rest, nil
}

// Init sets the paths and loads settings. dataDir is the --data-dir value (may be empty),
// embeddedEnv the .env built into the binary.
func Init(dataDir, embeddedEnv string) error {
	AppRoot, IsDev = appRoot()
	SharedEnvFile = filepath.Join(AppRoot, ".env")
	PrivateEnvFile = filepath.Join(AppRoot, ".env.private")

	// --data-dir, else AEX_DATA_DIR (real environment only: app settings live in the data folder),
	// else the per-user default (%APPDATA%\aex on Windows).
	if dataDir == "" {
		dataDir = os.Getenv("AEX_DATA_DIR")
	}
	if dataDir == "" {
		config, err := os.UserConfigDir()
		if err != nil {
			return err
		}
		dataDir = filepath.Join(config, "aex")
	}
	abs, err := filepath.Abs(dataDir)
	if err != nil {
		return err
	}
	DataDir = abs
	JiraExportDir = filepath.Join(DataDir, "output", "jira-export")
	ConfigEnvFile = filepath.Join(DataDir, ".env.config")

	openCredentials()
	load(embeddedEnv)
	return nil
}

func appRoot() (string, bool) {
	// Without -trimpath the compiler records this file's absolute path; use the checkout while it exists.
	if _, file, _, ok := runtime.Caller(0); ok && filepath.IsAbs(file) {
		root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
		if _, err := os.Stat(filepath.Join(root, "go.mod")); err == nil {
			return root, true
		}
	}
	// A release plugin (internal/plugin) runs from the plugins folder; aex tells it its own root.
	if root := os.Getenv(pluginAppRootVar); root != "" {
		return root, false
	}
	exe, err := os.Executable()
	if err != nil {
		return ".", false
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return filepath.Dir(exe), false
}
