package settings

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"aex/internal/secrets"
	"aex/internal/ui"
)

// Setting is something configure asks for. App settings (with Default) must pass Valid.
type Setting struct {
	Name    string
	Hint    string
	Secret  bool
	Default string
	Rule    string
	Valid   func(float64) bool
	// Credential is the key the setting is kept under in Credentials (the OS credential store)
	// instead of app settings.
	Credential string
}

// AuthSettings belong in app settings (secret ones in the credential store); .env and real env
// still work as fallbacks.
var AuthSettings = []Setting{
	{Name: "AEXT_EMAIL", Hint: "Email you log in to AEXT with"},
	{Name: "JIRA_EMAIL", Hint: "Email of your Atlassian (Jira) account"},
	{
		Name:       "JIRA_TOKEN",
		Hint:       "Jira Cloud API token: https://id.atlassian.com/manage-profile/security/api-tokens",
		Secret:     true,
		Credential: secrets.JiraToken,
	},
	// The OAuth client aex signs in to Google with: built in through .env, set here to use your own
	// (a "Desktop app" client of a Google Cloud project with the Calendar API enabled).
	{Name: "GOOGLE_CLIENT_ID", Hint: "OAuth client ID aex signs in to Google with (built in; set to use your own Desktop app client)"},
	{
		Name:       "GOOGLE_CLIENT_SECRET",
		Hint:       "Secret of that OAuth client (built in; set with your own GOOGLE_CLIENT_ID)",
		Secret:     true,
		Credential: secrets.GoogleClientSecret,
	},
}

// AppSettings are numeric settings with defaults. An invalid value falls back to the default, with a warning.
var AppSettings = []Setting{
	{
		Name:    "HOURS_PER_DAY",
		Default: "8",
		Hint:    "Working hours per day, used for the quota",
		Rule:    "a number above 0, at most 24",
		Valid:   func(n float64) bool { return n > 0 && n <= 24 },
	},
	{
		Name:    "TZ_OFFSET_HOURS",
		Default: "7",
		Hint:    "Timezone as a UTC offset in hours: 7 = UTC+7, -5 = UTC-5, 5.5 = UTC+5:30",
		Rule:    "a number from -12 to 14, in steps of 0.25",
		Valid:   func(n float64) bool { return n >= -12 && n <= 14 && n*4 == float64(int(n*4)) },
	},
}

// All is everything configure asks for.
var All = append(append([]Setting{}, AuthSettings...), AppSettings...)

var (
	mu          sync.RWMutex
	vars        map[string]string // merged from all layers, then changed by Set / Unset
	realEnv     map[string]string
	embeddedEnv string
	warned      = map[string]bool{}
)

// Layer is one source of settings.
type Layer struct {
	Label string
	Vars  map[string]string
}

// Layers lists where settings come from, lowest priority first. App settings (.env.config) and the
// credential store (secret settings) beat even real environment variables. Release builds carry
// the repo's .env inside.
func Layers() []Layer {
	var layers []Layer
	if !IsDev {
		layers = append(layers, Layer{".env built into the exe", ParseEnv(embeddedEnv)})
	}
	return append(layers,
		Layer{".env", readEnvFile(SharedEnvFile)},
		Layer{"environment variables", realEnv},
		Layer{"app settings", readEnvFile(ConfigEnvFile)},
		credentialLayer(),
	)
}

// userLayers is how many layers at the top of Layers the app writes: app settings, credential store.
const userLayers = 2

// Fallbacks are the layers below the ones the app writes.
func Fallbacks() []Layer {
	layers := Layers()
	return layers[:len(layers)-userLayers]
}

// InAppSettings reports whether the app saved a setting (app settings or credential store).
func InAppSettings(name string) bool {
	layers := Layers()
	for _, l := range layers[len(layers)-userLayers:] {
		if _, ok := l.Vars[name]; ok {
			return true
		}
	}
	return false
}

func readEnvFile(file string) map[string]string {
	data, err := os.ReadFile(file)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			ui.Warn("could not read %s: %v", file, err)
		}
		return map[string]string{}
	}
	return ParseEnv(string(data))
}

func load(embedded string) {
	embeddedEnv = embedded
	realEnv = map[string]string{}
	for _, kv := range os.Environ() {
		if k, v, ok := strings.Cut(kv, "="); ok && k != "" {
			realEnv[k] = v
		}
	}
	vars = map[string]string{}
	for _, l := range Layers() {
		for k, v := range l.Vars {
			if IsPlugin && IsSecret(k) && l.Label != Credentials.Name() {
				continue
			}
			vars[k] = v
		}
	}
	seedAppSettings()
}

// Get returns the current value of a setting, "" when unset.
func Get(name string) string {
	v, _ := lookup(name)
	return v
}

// Set changes a setting for this process only (SaveAppSettings persists).
func Set(name, value string) {
	mu.Lock()
	defer mu.Unlock()
	vars[name] = value
}

func Unset(name string) {
	mu.Lock()
	defer mu.Unlock()
	delete(vars, name)
}

func lookup(name string) (string, bool) {
	mu.RLock()
	defer mu.RUnlock()
	v, ok := vars[name]
	return v, ok
}

func appSetting(name string) *Setting {
	for i := range AppSettings {
		if AppSettings[i].Name == name {
			return &AppSettings[i]
		}
	}
	return nil
}

// Problem says why value is not valid for setting name, or "" when it is (or the setting has no rule).
func Problem(name, value string) string {
	def := appSetting(name)
	if def == nil {
		return ""
	}
	n, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err == nil && def.Valid(n) {
		return ""
	}
	return fmt.Sprintf("%s must be %s, got %q", name, def.Rule, value)
}

// Number returns an AppSettings value, or its default if missing or invalid (warning once).
func Number(name string) float64 {
	def := appSetting(name)
	value, ok := lookup(name)
	if !ok {
		value = def.Default
	} else if problem := Problem(name, value); problem != "" {
		mu.Lock()
		first := !warned[name]
		warned[name] = true
		mu.Unlock()
		if first {
			ui.Warn("%s (%s); using %s.", problem, ConfigEnvFile, def.Default)
		}
		value = def.Default
	}
	n, _ := strconv.ParseFloat(strings.TrimSpace(value), 64)
	return n
}

var plainValue = regexp.MustCompile(`^[\w.@+\-/:=]*$`)

// Unquoted when ParseEnv reads it back unchanged, else single-quoted (no escapes inside those).
func formatLine(name, value string) (string, error) {
	if plainValue.MatchString(value) {
		return name + "=" + value, nil
	}
	if strings.Contains(value, "'") {
		return "", fmt.Errorf("%s cannot contain a single quote", name)
	}
	return name + "='" + value + "'", nil
}

// setLine replaces or removes (value nil) name in the file text, keeping other lines and comments.
func setLine(text, name string, value *string) (string, error) {
	lines := regexp.MustCompile(`\r?\n`).Split(text, -1)
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	match := regexp.MustCompile(`^\s*(export\s+)?` + regexp.QuoteMeta(name) + `\s*=`)
	at := -1
	for i, l := range lines {
		if match.MatchString(l) {
			at = i
			break
		}
	}
	switch {
	case value == nil:
		if at >= 0 {
			lines = append(lines[:at], lines[at+1:]...)
		}
	default:
		line, err := formatLine(name, *value)
		if err != nil {
			return "", err
		}
		if at >= 0 {
			lines[at] = line
		} else {
			lines = append(lines, line)
		}
	}
	if len(lines) == 0 {
		return "", nil
	}
	return strings.Join(lines, "\n") + "\n", nil
}

const appSettingsHeader = "# aex app settings. Edit here or run \"aex configure\". Overrides .env and env vars.\n"

// Change sets Name to *Value, or removes it when Value is nil.
type Change struct {
	Name  string
	Value *string
}

// SaveAppSettings writes changes to app settings, keeping everything else in the file. Secret
// settings go to the credential store instead.
func SaveAppSettings(changes []Change) error {
	var file []Change
	for _, c := range changes {
		key := credentialKey(c.Name)
		if key == "" {
			file = append(file, c)
			continue
		}
		if err := saveCredential(c.Name, key, c.Value); err != nil {
			return err
		}
		file = append(file, Change{Name: c.Name}) // never also in the file
	}
	return writeAppSettingsFile(file)
}

func writeAppSettingsFile(changes []Change) error {
	text := appSettingsHeader
	if data, err := os.ReadFile(ConfigEnvFile); err == nil {
		text = string(data)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	for _, c := range changes {
		var err error
		if text, err = setLine(text, c.Name, c.Value); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(filepath.Dir(ConfigEnvFile), 0o755); err != nil {
		return err
	}
	return os.WriteFile(ConfigEnvFile, []byte(text), 0o600)
}

// Source returns the label of the source a setting's value comes from, "" when unset.
func Source(name string) string {
	layers := Layers()
	for i := len(layers) - 1; i >= 0; i-- {
		if layers[i].Vars[name] != "" {
			return layers[i].Label
		}
	}
	return ""
}

// Mask shows only the ends of a secret.
func Mask(value string) string {
	r := []rune(value)
	if len(r) > 8 {
		return string(r[:4]) + "…" + string(r[len(r)-4:])
	}
	return "****"
}

// RemoveAppSetting removes a setting from app settings (or the credential store) and falls back to the next source down, as a
// restart would. Returns that source's label, "" when none holds it.
func RemoveAppSetting(name string) (fallback string, err error) {
	if err := SaveAppSettings([]Change{{Name: name}}); err != nil {
		return "", err
	}
	Unset(name)
	layers := Fallbacks()
	for i := len(layers) - 1; i >= 0; i-- {
		if v := layers[i].Vars[name]; v != "" {
			Set(name, v)
			return layers[i].Label, nil
		}
	}
	return "", nil
}

// First start (or a setting added since): write AppSettings to app settings, so they are there to
// edit. Uses the value already in effect from a fallback source when valid, else the default.
func seedAppSettings() {
	saved := readEnvFile(ConfigEnvFile)
	var changes []Change
	for _, s := range AppSettings {
		if _, ok := saved[s.Name]; ok {
			continue
		}
		value := s.Default
		if current, ok := lookup(s.Name); ok && Problem(s.Name, current) == "" {
			value = current
		}
		changes = append(changes, Change{s.Name, &value})
	}
	if len(changes) == 0 {
		return
	}
	if err := SaveAppSettings(changes); err != nil {
		ui.Warn("could not write defaults to %s: %v", ConfigEnvFile, err)
		return
	}
	for _, c := range changes {
		Set(c.Name, *c.Value)
	}
}

// Require returns a setting that must be present (base URLs in .env).
func Require(name string) (string, error) {
	if v := Get(name); v != "" {
		return v, nil
	}
	return "", fmt.Errorf("missing required setting %s (set it in .env)", name)
}

// RequireAuth returns an auth setting; if missing, prompts for it and offers to save it to app settings.
func RequireAuth(name string) (string, error) {
	if v := Get(name); v != "" {
		return v, nil
	}
	if IsPlugin && IsSecret(name) {
		return "", fmt.Errorf("%s was not passed to this plugin (it has to ask for access, see plugin.Main)", name)
	}
	var def Setting
	for _, s := range AuthSettings {
		if s.Name == name {
			def = s
		}
	}
	if err := ui.AssertInteractive(fmt.Sprintf("Missing %s (run configure to set it); prompting for it", name)); err != nil {
		return "", err
	}

	fmt.Fprintln(os.Stderr, ui.Err.Yellow(name+" is not set."))
	value, err := ui.Input(ui.Field{
		Title:       name,
		Description: def.Hint + ". configure sets all login settings at once.",
		Secret:      def.Secret,
		Validate:    ui.Required(name),
	})
	if err != nil {
		return "", err
	}
	Set(name, value)

	save, err := ui.Confirm(fmt.Sprintf("Save %s to app settings?", name), true)
	if err != nil {
		return "", err
	}
	if save {
		if err := SaveAppSettings([]Change{{name, &value}}); err != nil {
			return "", err
		}
		fmt.Fprintln(os.Stderr, ui.Err.Green("Saved to "+ConfigEnvFile))
	}
	return value, nil
}
