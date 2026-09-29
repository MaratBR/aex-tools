// pull-all, a cm-release tool: fetches and pulls master, PROD and staging in every CM repo,
// finishing on staging.
package main

import (
	"flag"

	"aex/internal/tool"
)

const pullUsage = `Usage: cm-release pull-all

Checks every repo is clean (asks to fix and recheck if not), then in each one fetches (--all
--prune) and pulls master, PROD and staging, finishing on staging.

Repos are <reposDir>/<repo> for the reposDir and repos settings in
<data folder>/plugin-settings/cm-release.json (the folder is asked for on first run).`

func runPull(args []string) error {
	fs := flag.NewFlagSet("pull-all", flag.ContinueOnError)
	if done, err := tool.ParseFlags(fs, args, pullUsage); done || err != nil {
		return err
	}
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
	for _, r := range rs {
		printSection(r)
		printStep("Fetching...")
		if _, err := r.git("fetch", "--all", "--prune"); err != nil {
			return err
		}
		for _, b := range []string{masterBranch, prodBranch, stagingBranch} {
			printStep("Pulling " + b + "...")
			if _, err := r.git("checkout", b); err != nil {
				return err
			}
			if _, err := r.git("pull", "origin", b); err != nil {
				return err
			}
		}
		printStep("Checking out " + stagingBranch + "...")
		if _, err := r.git("checkout", stagingBranch); err != nil {
			return err
		}
	}
	printDone("\nDone. All repos fetched/pulled (master, PROD, staging) and checked out on staging.")
	return nil
}
