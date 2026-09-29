// merge-prod, a cm-release tool: merges the release/VERSION branch every CM repo is on into PROD,
// tags the merge commit vVERSION, and optionally pushes PROD and the tag.
package main

import (
	"errors"
	"flag"
	"fmt"
	"strings"

	"aex/internal/tool"
	"aex/internal/ui"
)

const mergeUsage = `Usage: cm-release merge-prod

Detects VERSION from the release/VERSION branch every repo is on (all must be on the same one),
asks you to confirm it, then validates each repo: fetched, on release/VERSION, clean, local PROD
equal to origin/PROD. An existing vVERSION tag is shown and, if you agree, deleted (locally and on
origin) so it can be made again on the merge commit.

After you confirm, merges release/VERSION into PROD (--no-ff) and tags the merge commit vVERSION
in each repo, then asks whether to push PROD and the tag to origin.`

func runMerge(args []string) error {
	fs := flag.NewFlagSet("merge-prod", flag.ContinueOnError)
	if done, err := tool.ParseFlags(fs, args, mergeUsage); done || err != nil {
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

	bs, err := branches(rs)
	if err != nil {
		return err
	}
	version, err := releaseVersion(branchNames(bs))
	if err != nil {
		printBranches("Current branches:", bs)
		return fmt.Errorf("could not detect VERSION: %w", err)
	}
	branch, tag := releaseBranch(version), releaseTag(version)
	fmt.Println(out.Cyan("\nDetected release branch '" + branch + "' in all repos."))
	if ok, err := ui.Confirm(fmt.Sprintf("Is '%s' the correct VERSION to merge into %s?", version, prodBranch), true); err != nil || !ok {
		if err == nil {
			err = errors.New("cancelled")
		}
		return err
	}
	fmt.Println(out.Cyan(fmt.Sprintf("\nMerging '%s' into '%s', tag '%s'.", branch, prodBranch, tag)))

	for _, r := range rs {
		if err := validateMerge(r, branch, tag); err != nil {
			return err
		}
	}

	merge, err := ui.Confirm(fmt.Sprintf("\nAll repos validated. Merge '%s' into '%s' in each repo?", branch, prodBranch), false)
	if err != nil {
		return err
	}
	if !merge {
		return errors.New("cancelled")
	}
	for _, r := range rs {
		printSection(r)
		printStep("Checking out " + prodBranch + "...")
		if _, err := r.git("checkout", prodBranch); err != nil {
			return err
		}
		printStep("Merging " + branch + " into " + prodBranch + "...")
		if _, err := r.git("merge", "--no-ff", branch); err != nil {
			return err
		}
		printStep("Tagging merge commit as " + tag + "...")
		if _, err := r.git("tag", tag); err != nil {
			return err
		}
		printDone("Merged and tagged in " + r.name + ".")
	}

	push, err := ui.Confirm(fmt.Sprintf("\nMerges complete. Push '%s' and tag '%s' to origin in all repos?", prodBranch, tag), false)
	if err != nil {
		return err
	}
	if !push {
		printDone(fmt.Sprintf("\nDone. Release '%s' merged into '%s' and tagged '%s' locally in all repos. Not pushed.", version, prodBranch, tag))
		return nil
	}
	for _, r := range rs {
		printSection(r)
		printStep("Pushing " + prodBranch + "...")
		if _, err := r.git("push", "origin", prodBranch); err != nil {
			return err
		}
		printStep("Pushing tag " + tag + "...")
		if _, err := r.git("push", "origin", tag); err != nil {
			return err
		}
		printDone(fmt.Sprintf("Pushed %s and %s in %s.", prodBranch, tag, r.name))
	}
	printDone(fmt.Sprintf("\nDone. Release '%s' merged into '%s', tagged '%s', and pushed in all repos.", version, prodBranch, tag))
	return nil
}

// validateMerge checks the repo is ready to merge branch into PROD, removing a tag left in the way
// if the user agrees.
func validateMerge(r repo, branch, tag string) error {
	printSection(r)
	printStep("Fetching origin...")
	if _, err := r.git("fetch", "origin"); err != nil {
		return err
	}

	printStep("Checking current branch...")
	current, err := r.branch()
	if err != nil {
		return err
	}
	if current != branch {
		return fmt.Errorf("in %s: expected to be on branch '%s' but found '%s'", r.path, branch, current)
	}

	printStep("Checking working tree is clean...")
	if d, err := r.dirty(); err != nil || d {
		if err != nil {
			return err
		}
		return fmt.Errorf("in %s: working tree is not clean, commit, stash, or discard changes first", r.path)
	}

	printStep("Checking " + prodBranch + " branch exists and is up to date...")
	if s, err := r.git("branch", "--list", prodBranch); err != nil || s == "" {
		if err != nil {
			return err
		}
		return fmt.Errorf("in %s: local branch '%s' does not exist", r.path, prodBranch)
	}
	if s, err := r.git("branch", "-r", "--list", "origin/"+prodBranch); err != nil || s == "" {
		if err != nil {
			return err
		}
		return fmt.Errorf("in %s: remote branch 'origin/%s' does not exist", r.path, prodBranch)
	}
	local, err := r.git("rev-parse", prodBranch)
	if err != nil {
		return err
	}
	remote, err := r.git("rev-parse", "origin/"+prodBranch)
	if err != nil {
		return err
	}
	if local != remote {
		return fmt.Errorf("in %s: local '%s' (%s) is not up to date with 'origin/%s' (%s), pull it first", r.path, prodBranch, local, prodBranch, remote)
	}

	printStep("Checking tag '" + tag + "' is not already in place...")
	if s, err := r.git("tag", "--list", tag); err != nil || s == "" {
		if err == nil {
			printDone("Validation OK for " + r.name + ".")
		}
		return err
	}
	tagged, err := r.git("rev-list", "-n", "1", tag)
	if err != nil {
		return err
	}
	info, err := r.git("log", "-1", "--format=%H%n%an <%ae>%n%ad%n%s", tag)
	if err != nil {
		return err
	}
	commit := strings.SplitN(info, "\n", 4)
	for len(commit) < 4 {
		commit = append(commit, "")
	}
	out := ui.Out
	fmt.Println(out.Yellow(fmt.Sprintf("\nTag '%s' already exists in %s, but merge-prod tags the merge commit AFTER merging into %s.", tag, r.name, prodBranch)))
	if tagged == local {
		fmt.Println(out.Yellow(fmt.Sprintf("It points at the tip of '%s' (%s): likely left over from a previous run.", prodBranch, tagged)))
	} else {
		fmt.Println(out.Yellow(fmt.Sprintf("It points at commit %s, which is NOT the tip of '%s' (%s): it looks like it was tagged in the wrong place.", tagged, prodBranch, local)))
	}
	fmt.Printf("  Hash:    %s\n  Author:  %s\n  Date:    %s\n  Subject: %s\n", commit[0], commit[1], commit[2], commit[3])
	del, err := ui.Confirm(fmt.Sprintf("Delete existing tag '%s' in %s so it can be recreated on the merge commit?", tag, r.name), false)
	if err != nil {
		return err
	}
	if !del {
		return fmt.Errorf("tag '%s' in %s is in an unexpected place and was not removed, resolve manually first", tag, r.path)
	}
	printStep("Deleting local tag '" + tag + "'...")
	if _, err := r.git("tag", "-d", tag); err != nil {
		return err
	}
	remoteTag, err := r.git("ls-remote", "--tags", "origin", tag)
	if err != nil {
		return err
	}
	if remoteTag != "" {
		printStep("Deleting remote tag '" + tag + "'...")
		if _, err := r.git("push", "origin", ":refs/tags/"+tag); err != nil {
			return err
		}
	}
	printDone("Validation OK for " + r.name + ".")
	return nil
}
