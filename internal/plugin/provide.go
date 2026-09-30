package plugin

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
)

// Provides: a plugin can give aex data on request, such as the git repos it works on, which the Git
// status widget offers as its default list (internal/webui/gitstatus.go):
//
//	<plugin> --aex-provide <name>  prints {"result": ...} or {"error": "..."}
//
// --aex-describe lists the names ("provides": ["git-repos"]). aex runs it only while the plugin is
// safe, never asking anything, with the plugin's settings but no access (no credentials).

const provideFlag = "--aex-provide"

// GitRepos is the Provide of the git repos a plugin works on: a list of their folders (full paths).
const GitRepos = "git-repos"

// Provide is data a plugin gives aex (see Main): Run gives it, marshalled to JSON, with no way to
// ask questions.
type Provide struct {
	Name string
	Run  func() (any, error)
}

// serveProvide answers --aex-provide (args after the flag) for Main.
func serveProvide(args []string, ps []Provide) error {
	if len(args) == 0 {
		return fmt.Errorf("%s needs a name", provideFlag)
	}
	reply := map[string]any{}
	if i := slices.IndexFunc(ps, func(p Provide) bool { return p.Name == args[0] }); i < 0 {
		reply["error"] = fmt.Sprintf("this plugin provides no %q", args[0])
	} else if result, err := ps[i].Run(); err != nil {
		reply["error"] = err.Error()
	} else {
		reply["result"] = result
	}
	return json.NewEncoder(os.Stdout).Encode(reply)
}

// Provided gives what the plugin provides as name (Info.Provides). The plugin must be safe.
func Provided(p Info, name string) (json.RawMessage, error) {
	if !slices.Contains(p.Provides, name) {
		return nil, fmt.Errorf("plugin %s provides no %q", p.Name, name)
	}
	o, err := openSafe(p.Name, p.Path)
	if err != nil {
		return nil, err
	}
	env, err := widgetEnv(nil)
	if err != nil {
		o.close()
		return nil, err
	}
	out, err := runWidget(o, p.Path, env, nil, provideFlag, name)
	if err != nil {
		return nil, err
	}
	return callResult(provideFlag, out)
}
