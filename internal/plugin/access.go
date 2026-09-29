package plugin

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"

	"aex/internal/settings"
	"aex/internal/ui"
)

// Access is something a plugin asks aex for, in its --aex-describe output. A plugin gets no
// credentials except those of the access the user granted it; the grant is kept (in the credential
// store) for the file's hash, so a changed file asks again.
//
// This keeps honest plugins to what they need and shows the user what that is. It is no sandbox: a
// plugin runs with the user's rights, which is why its file must be approved first (trust.go).
type Access string

const Jira Access = "jira"

type accessInfo struct {
	label    string
	settings []string // settings aex makes sure are set; the secret ones are passed to the plugin
}

var accessKinds = map[Access]accessInfo{
	Jira: {"Jira: your Jira email and API token", []string{"JIRA_EMAIL", "JIRA_TOKEN"}},
}

// grantKey is the credential store key of the access granted to the plugin at path.
func grantKey(path string) string { return "plugin-access:" + normPath(path) }

// grant makes sure the user granted the plugin its access, asking if not, and returns the secret
// settings to pass it (asking for any that are not set).
func grant(name, path, hash string, access []Access) ([]string, error) {
	if len(access) == 0 {
		return nil, nil
	}
	access = slices.Compact(slices.Sorted(slices.Values(access)))
	names := make([]string, len(access))
	for i, a := range access {
		if _, ok := accessKinds[a]; !ok {
			return nil, fmt.Errorf("plugin %s asks for unknown access %q", name, a)
		}
		names[i] = string(a)
	}
	want := hash + " " + strings.Join(names, ",")
	granted, err := settings.Credentials.Get(grantKey(path))
	if err != nil {
		return nil, err
	}
	if granted != want {
		if err := askAccess(name, access); err != nil {
			return nil, err
		}
		if err := settings.Credentials.Set(grantKey(path), want); err != nil {
			return nil, err
		}
		if err := setApproved(path, true); err != nil {
			return nil, err
		}
	}

	var pass []string
	for _, a := range access {
		for _, s := range accessKinds[a].settings {
			if _, err := settings.RequireAuth(s); err != nil {
				return nil, err
			}
			if settings.IsSecret(s) {
				pass = append(pass, s)
			}
		}
	}
	return pass, nil
}

func askAccess(name string, access []Access) error {
	if err := ui.AssertInteractive("granting plugin " + name + " access"); err != nil {
		return err
	}
	c := ui.Err
	fmt.Fprintf(os.Stderr, "%s Plugin %s asks for access to:\n", c.Bold(c.Yellow("▲")), c.Bold(name))
	for _, a := range access {
		fmt.Fprintf(os.Stderr, "  • %s\n", accessKinds[a].label)
	}
	yes, err := ui.Confirm("Allow?", false)
	if err != nil {
		return err
	}
	if !yes {
		return errors.New("plugin " + name + " was not granted access")
	}
	return nil
}
