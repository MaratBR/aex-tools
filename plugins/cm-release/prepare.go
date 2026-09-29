// prepare-release, a cm-release tool: creates release/VERSION off staging in every CM repo, shows
// each one's diff against PROD, then pushes the branches. Tagging is left to merge-prod.
package main

import (
	"errors"
	"flag"
	"fmt"
	"strings"

	"aex/internal/tool"
	"aex/internal/ui"
)

const prepareUsage = `Usage: cm-release prepare-release [--version <VERSION>]

Checks every repo is clean, then creates release/VERSION off staging in each one. When all repos
are already on the same release/VERSION branch, offers to use it instead. Shows each repo's diff
stat PROD...release/VERSION, one at a time, then (after you confirm) pushes release/VERSION to
origin. The vVERSION tag is made later, by merge-prod.

  --version   Release VERSION, e.g. 4.5 (asked for when left out)`

func runPrepare(args []string) error {
	fs := flag.NewFlagSet("prepare-release", flag.ContinueOnError)
	versionFlag := fs.String("version", "", "")
	if done, err := tool.ParseFlags(fs, args, prepareUsage); done || err != nil {
		return err
	}
	out := ui.Out
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	rs, err := repos(cfg)
	if err != nil {
		return err
	}
	if err := waitForClean(rs); err != nil {
		return err
	}

	bs, err := branches(rs)
	if err != nil {
		return err
	}
	version, create := strings.TrimSpace(*versionFlag), true
	detected, err := releaseVersion(branchNames(bs))
	switch {
	case errors.Is(err, errNoneOnRelease):
	case errors.Is(err, errNotAllRelease):
		printBranches("Repos are on inconsistent branches:", bs)
		return errors.New("not all repos are on a release branch, resolve manually first")
	case err != nil:
		printBranches("Repos are on different release branches:", bs)
		return errors.New("resolve manually first")
	case version == "" || version == detected:
		fmt.Println(out.Yellow("\nAll repos are already on branch '" + releaseBranch(detected) + "'."))
		use, err := ui.Confirm("Is '"+detected+"' the correct version to prepare?", true)
		if err != nil {
			return err
		}
		if use {
			version, create = detected, false
			break
		}
		fresh, err := ui.Confirm("Checkout "+stagingBranch+" and start a new release branch instead?", false)
		if err != nil {
			return err
		}
		if !fresh {
			return errors.New("cancelled")
		}
		version = ""
	}
	if version == "" {
		if version, err = ui.Input(ui.Field{Title: "Release VERSION", Placeholder: "e.g. 4.5", Validate: ui.Required("VERSION")}); err != nil {
			return err
		}
	}
	branch := releaseBranch(version)

	if create {
		fmt.Println(out.Cyan("\nWill create branch '" + branch + "' off " + stagingBranch + " in each repo."))
		for _, r := range rs {
			printSection(r)
			printStep("Checking out " + stagingBranch + "...")
			if _, err := r.git("checkout", stagingBranch); err != nil {
				return err
			}
			printStep("Creating " + branch + "...")
			if _, err := r.git("checkout", "-b", branch); err != nil {
				return err
			}
		}
	} else {
		fmt.Println(out.Cyan("\nUsing existing branch '" + branch + "' in each repo."))
	}

	fmt.Println(out.Cyan("\nReviewing diffs against " + prodBranch + ", one repo at a time."))
	pressAnyKey("start reviewing diffs")
	diffArgs := []string{"--no-pager", "diff", "--stat"}
	if out.On() {
		diffArgs = append(diffArgs, "--color=always")
	}
	for _, r := range rs {
		stat, err := r.git(append(diffArgs, prodBranch+"..."+branch)...)
		if err != nil {
			return err
		}
		if stat == "" {
			fmt.Println(out.Dim(fmt.Sprintf("Skipping %s (no diff between %s and %s).", r.name, prodBranch, branch)))
			continue
		}
		ui.ClearScreen()
		fmt.Printf("%s\n\n%s\n\n", out.Bold(out.Cyan(fmt.Sprintf("=== Diff for %s: %s -> %s ===", r.name, prodBranch, branch))), stat)
		pressAnyKey("continue to the next repo")
	}

	ui.ClearScreen()
	push, err := ui.Confirm(fmt.Sprintf("All diffs reviewed for release '%s'. Push branch '%s' to origin?", version, branch), false)
	if err != nil {
		return err
	}
	if !push {
		fmt.Println(out.Yellow("Not pushed: branch '" + branch + "' is left local."))
		return nil
	}
	for _, r := range rs {
		b, err := r.branch()
		if err != nil {
			return err
		}
		if b != branch {
			return fmt.Errorf("sanity check failed in %s: expected branch '%s' but found '%s'", r.path, branch, b)
		}
		if d, err := r.dirty(); err != nil || d {
			if err != nil {
				return err
			}
			return fmt.Errorf("sanity check failed in %s: working tree is not clean", r.path)
		}
	}
	for _, r := range rs {
		printSection(r)
		printStep("Pushing " + branch + "...")
		if _, err := r.git("push", "origin", branch); err != nil {
			return err
		}
	}
	printDone(fmt.Sprintf("\nDone. Release '%s' branch pushed to origin in all repos. Tag '%s' will be created when this release is merged into %s (merge-prod).",
		version, releaseTag(version), prodBranch))
	return nil
}
