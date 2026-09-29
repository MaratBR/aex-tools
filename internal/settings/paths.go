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
	// Debug is set by --debug (bin\aex.ps1 passes it): tools and widgets for developing aex
	// (tool.Tool.Debug, the window's debug widgets) can be used only then.
	Debug bool
	// FakeError is set by --debug-fake-error: the window prints a made-up error a few seconds after
	// it opens, outside any run, to see how it shows one (as a widget's call warning would).
	FakeError bool
	// AppRoot holds .env: the repo for a dev build, else the folder holding the exe.
	AppRoot string
	// DataDir holds app settings (.env.config), the AEXT session and output files.
	DataDir string
	// DataDirFromArg is true when DataDir came from --data-dir, which a launcher has to pass on.
	DataDirFromArg bool
	JiraExportDir  string
	// OutputDir holds files tools write for the user, one folder per tool.
	OutputDir string
	// PluginSettingsDir holds plugins' settings files, one per plugin.
	PluginSettingsDir string
	// CustomToolsFile lists the custom tools added (internal/custom): scripts run by a tool adapter.
	CustomToolsFile string
	// HomeFile holds the window's home page: its widgets, in order, with their sizes.
	HomeFile string
	// OnboardedFile is there once the window's onboarding (logging in to each service) was done or
	// skipped: the window opens it on first start only.
	OnboardedFile string
	SharedEnvFile string
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

	// --data-dir, else AEX_DATA_DIR (real environment only: app settings live in the data folder),
	// else the per-user default (%APPDATA%\aex on Windows).
	DataDirFromArg = dataDir != ""
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
	OutputDir = filepath.Join(DataDir, "output")
	JiraExportDir = filepath.Join(OutputDir, "jira-export")
	PluginSettingsDir = filepath.Join(DataDir, "plugin-settings")
	CustomToolsFile = filepath.Join(DataDir, "custom-tools.json")
	HomeFile = filepath.Join(DataDir, "home.json")
	OnboardedFile = filepath.Join(DataDir, "onboarded")
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
