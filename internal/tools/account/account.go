// Package account is the account tool: who you are logged in as in AEXT and Jira, logging in and out.
package account

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"aex/internal/aext"
	"aex/internal/jira"
	"aex/internal/settings"
	"aex/internal/tool"
	"aex/internal/ui"
)

func help() string {
	return fmt.Sprintf(`Usage: account [--show | --login aext|jira | --logout aext|jira | --wipe-session]

Shows who you are logged in as in AEXT and Jira, with details, then offers actions. Without a flag
on a terminal, loops over details and actions until done; elsewhere only shows details.

  --show          Only show details
  --login aext    Log in to AEXT with a new emailed code (replaces the cached session)
  --login jira    Set JIRA_EMAIL and JIRA_TOKEN, checked against Jira before saving them
  --logout aext   End the AEXT session on the server, then delete it locally
  --logout jira   Delete the saved JIRA_TOKEN
  --wipe-session  Delete the saved AEXT session without telling the server

The AEXT session and JIRA_TOKEN are kept in %s.
Logging out, wiping the session and replacing a login ask for confirmation first.`, settings.Credentials.Name())
}

// Tool is the account tool.
var Tool = tool.Tool{Name: "account", Summary: "Show AEXT / Jira login details; log in, log out, wipe the session", Run: run}

func run(args []string) error {
	fs := flag.NewFlagSet("account", flag.ContinueOnError)
	show := fs.Bool("show", false, "")
	login := fs.String("login", "", "")
	logout := fs.String("logout", "", "")
	wipe := fs.Bool("wipe-session", false, "")
	if done, err := tool.ParseFlags(fs, args, help()); done || err != nil {
		return err
	}
	var actions []string
	if *show {
		actions = append(actions, "show")
	}
	if *login != "" {
		actions = append(actions, "login "+*login)
	}
	if *logout != "" {
		actions = append(actions, "logout "+*logout)
	}
	if *wipe {
		actions = append(actions, "wipe aext")
	}
	if len(actions) > 1 {
		return errors.New("use only one of --show, --login, --logout, --wipe-session")
	}
	action := ""
	if len(actions) == 1 {
		action = actions[0]
	}

	c, err := aext.New()
	if err != nil {
		return err
	}
	switch action {
	case "":
		if !ui.IsInteractive() {
			printDetails(c)
			return nil
		}
		return loop(c)
	case "show":
		printDetails(c)
		return nil
	}
	if !slices.Contains(actionNames, action) {
		return fmt.Errorf("unknown service in --%s (use aext or jira)", strings.Fields(action)[0])
	}
	if err := ui.AssertInteractive("account " + action); err != nil {
		return err
	}
	return perform(c, action)
}

var actionNames = []string{"login aext", "logout aext", "wipe aext", "login jira", "logout jira"}

func loop(c *aext.Client) error {
	for {
		a, j := printDetails(c)
		var options []ui.Option
		if a.me == nil {
			options = append(options, ui.Option{Label: "Log in to AEXT", Value: "login aext"})
		} else {
			options = append(options, ui.Option{Label: "Log in to AEXT again (new code)", Value: "login aext"})
		}
		if c.HasSession() {
			options = append(options,
				ui.Option{Label: "Log out of AEXT", Value: "logout aext"},
				ui.Option{Label: "Wipe AEXT session (local only)", Value: "wipe aext"})
		}
		if j.me == nil {
			options = append(options, ui.Option{Label: "Set Jira login", Value: "login jira"})
		} else {
			options = append(options, ui.Option{Label: "Change Jira login", Value: "login jira"})
		}
		if settings.Get("JIRA_TOKEN") != "" {
			options = append(options, ui.Option{Label: "Remove Jira token", Value: "logout jira"})
		}
		options = append(options, ui.Option{Label: "Refresh", Value: "refresh"}, ui.Option{Label: "Done", Value: "done"})

		choice, err := ui.Choose("Account", options)
		if err != nil {
			return err
		}
		switch choice {
		case "done":
			return nil
		case "refresh":
		default:
			if err := perform(c, choice); err != nil {
				ui.Warn("%v", err)
			}
		}
		fmt.Println()
	}
}

func perform(c *aext.Client, action string) error {
	switch action {
	case "login aext":
		return loginAEXT(c)
	case "logout aext":
		return logoutAEXT(c)
	case "wipe aext":
		return wipeAEXT(c)
	case "login jira":
		return loginJira()
	case "logout jira":
		return logoutJira()
	}
	return fmt.Errorf("unknown action %q", action)
}

func confirm(question string) (bool, error) {
	yes, err := ui.Confirm(question, false)
	if err == nil && !yes {
		fmt.Println("Cancelled.")
	}
	return yes, err
}

func loginAEXT(c *aext.Client) error {
	if me, err := c.WhoAmI(timeout); err == nil && me != nil {
		yes, err := confirm(fmt.Sprintf("Logged in to AEXT as %s. Replace the session with a new login?", *me.Email))
		if err != nil || !yes {
			return err
		}
	}
	return c.Login()
}

func logoutAEXT(c *aext.Client) error {
	if !c.HasSession() {
		fmt.Println("No AEXT session, nothing to log out of.")
		return nil
	}
	yes, err := confirm("Log out of AEXT? The server ends the session and it is deleted from " + settings.Credentials.Name() + ".")
	if err != nil || !yes {
		return err
	}
	expired, err := c.Logout()
	if err != nil {
		return err
	}
	if expired {
		fmt.Println(ui.Out.Green("AEXT session had already expired; deleted it locally."))
	} else {
		fmt.Println(ui.Out.Green("Logged out of AEXT."))
	}
	return nil
}

func wipeAEXT(c *aext.Client) error {
	if !c.HasSession() {
		fmt.Println("No AEXT session, nothing to wipe.")
		return nil
	}
	yes, err := confirm("Delete the AEXT session from " + settings.Credentials.Name() + "? The server session stays valid until it expires; use log out to end it.")
	if err != nil || !yes {
		return err
	}
	if err := c.WipeSession(); err != nil {
		return err
	}
	fmt.Println(ui.Out.Green("AEXT session wiped; the next AEXT tool will ask for a login code."))
	return nil
}

func loginJira() error {
	base, err := settings.Require("JIRA_BASE_URL")
	if err != nil {
		return err
	}
	current := settings.Get("JIRA_EMAIL")
	email, err := ui.Input(ui.Field{
		Title:       "JIRA_EMAIL",
		Description: "Email of your Atlassian (Jira) account" + orKeep(current),
		Placeholder: current,
	})
	if err != nil {
		return err
	}
	if email == "" {
		email = current
	}
	if email == "" {
		return errors.New("JIRA_EMAIL is required")
	}
	token, err := ui.Input(ui.Field{
		Title:       "JIRA_TOKEN",
		Description: "Jira Cloud API token: https://id.atlassian.com/manage-profile/security/api-tokens",
		Secret:      true,
		Validate:    ui.Required("token"),
	})
	if err != nil {
		return err
	}

	me, err := jira.Check(base, email, token, 15*time.Second)
	if err != nil {
		return err
	}
	if me == nil {
		return errors.New("Jira rejected this email and token; nothing saved")
	}
	fmt.Printf("Jira accepts it: %s\n", ui.Out.Bold(me.DisplayName))
	if old := settings.Get("JIRA_TOKEN"); old != "" && old != token {
		yes, err := confirm("Replace the current Jira login?")
		if err != nil || !yes {
			return err
		}
	}
	if err := settings.SaveAppSettings([]settings.Change{{Name: "JIRA_EMAIL", Value: &email}, {Name: "JIRA_TOKEN", Value: &token}}); err != nil {
		return err
	}
	settings.Set("JIRA_EMAIL", email)
	settings.Set("JIRA_TOKEN", token)
	fmt.Println(ui.Out.Green(fmt.Sprintf("Saved JIRA_EMAIL to %s, JIRA_TOKEN to %s", settings.StoredIn("JIRA_EMAIL"), settings.StoredIn("JIRA_TOKEN"))))
	return nil
}

func orKeep(current string) string {
	if current == "" {
		return ""
	}
	return "\nEmpty keeps " + current
}

func logoutJira() error {
	source := settings.Source("JIRA_TOKEN")
	if source == "" {
		fmt.Println("No Jira token set, nothing to remove.")
		return nil
	}
	if !settings.InAppSettings("JIRA_TOKEN") {
		return fmt.Errorf("JIRA_TOKEN comes from %s, which aex does not manage; remove it there", source)
	}
	yes, err := confirm("Delete JIRA_TOKEN from " + settings.StoredIn("JIRA_TOKEN") + "? Jira tools will ask for it again.")
	if err != nil || !yes {
		return err
	}
	fallback, err := settings.RemoveAppSetting("JIRA_TOKEN")
	if err != nil {
		return err
	}
	fmt.Println(ui.Out.Green("Deleted JIRA_TOKEN from " + source))
	if fallback != "" {
		ui.Warn("JIRA_TOKEN is also set in %s, which now applies.", fallback)
	}
	return nil
}

const timeout = 10 * time.Second

type aextStatus struct {
	me  *aext.Me
	err error
}

type jiraStatus struct {
	me  *jira.Me
	err error
}

// printDetails checks both services in parallel and prints what they say about you.
func printDetails(c *aext.Client) (aextStatus, jiraStatus) {
	var a aextStatus
	var j jiraStatus
	var wg sync.WaitGroup
	wg.Go(func() { a.me, a.err = c.WhoAmI(timeout) })
	wg.Go(func() { j.me, j.err = jira.WhoAmI(timeout) })
	fmt.Println(ui.Out.Dim("Checking AEXT and Jira…"))
	wg.Wait()

	out := ui.Out
	store := settings.Credentials.Name()
	if settings.CredentialsNote != "" {
		store += out.Dim(" (" + settings.CredentialsNote + ")")
	}
	fmt.Printf("%s %s\n\n", out.Dim("Credentials"), store)
	fmt.Println(out.Bold("AEXT"))
	switch {
	case a.err != nil:
		field("Status", out.Yellow("check failed: "+a.err.Error()))
	case a.me != nil:
		field("Status", out.Green("logged in"))
		field("Name", a.me.DisplayName)
		field("User name", a.me.UserName)
		field("Email", *a.me.Email)
		for _, k := range extraFields(a.me.Fields) {
			field(k, a.me.Fields[k])
		}
	case c.HasSession():
		field("Status", out.Yellow("session expired"))
	default:
		field("Status", out.Yellow("not logged in"))
	}
	field("Server", settings.Get("AEXT_BASE_URL"))
	session := "none"
	if c.HasSession() {
		session = "saved"
	}
	field("Session", session)
	setting("AEXT_EMAIL")
	fmt.Println()

	fmt.Println(out.Bold("Jira"))
	switch {
	case errors.Is(j.err, jira.ErrNotConfigured):
		field("Status", out.Yellow("login not set"))
	case j.err != nil:
		field("Status", out.Yellow("check failed: "+j.err.Error()))
	case j.me == nil:
		field("Status", out.Yellow("token rejected"))
	default:
		field("Status", out.Green("logged in"))
		field("Name", j.me.DisplayName)
		field("Email", j.me.EmailAddress)
		field("Account ID", j.me.AccountID)
		field("Account type", j.me.AccountType)
		field("Active", j.me.Active)
		field("Time zone", j.me.TimeZone)
		field("Locale", j.me.Locale)
	}
	field("Server", settings.Get("JIRA_BASE_URL"))
	setting("JIRA_EMAIL")
	setting("JIRA_TOKEN")
	fmt.Println()
	return a, j
}

func field(label string, value any) {
	s := fmt.Sprint(value)
	if s == "" {
		return
	}
	fmt.Printf("  %s %s\n", ui.Out.Dim(fmt.Sprintf("%-13s", label)), s)
}

// setting shows an auth setting's value (secrets masked) and where it comes from.
func setting(name string) {
	v := settings.Get(name)
	if v == "" {
		field(name, ui.Out.Yellow("not set"))
		return
	}
	if name == "JIRA_TOKEN" {
		v = settings.Mask(v)
	}
	field(name, v+ui.Out.Dim(" ("+settings.Source(name)+")"))
}

var (
	shownFields = []string{"email", "display_name", "user_name"}
	secretKey   = regexp.MustCompile(`(?i)token|secret|password|hash|cookie`)
)

// extraFields lists the /api/auth/me fields not shown already, minus anything that looks secret.
func extraFields(fields map[string]any) []string {
	var keys []string
	for k, v := range fields {
		if v == nil || slices.Contains(shownFields, k) || secretKey.MatchString(k) {
			continue
		}
		switch v.(type) {
		case map[string]any, []any:
			b, _ := json.Marshal(v)
			if len(b) > 100 {
				b = append(b[:100], "…"...)
			}
			fields[k] = string(b)
		}
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}
