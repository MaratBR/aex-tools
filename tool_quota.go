package main

import (
	"flag"
	"fmt"

	"aex/internal/aext"
	"aex/internal/dates"
	"aex/internal/quota"
)

const quotaHelp = `Usage: quota [--verbose]

Shows AEXT hours quota for this month and last month: whether you are on track, working days
without hours, hours logged today, and hours/day needed for the rest of this month.

  --verbose  List leave warnings (unapproved or ignored leaves, leaves created by someone else)

Quota settings: HOURS_PER_DAY app setting (see configure), WorkingDaysCountry in internal/settings/config.go.`

func quotaTool(args []string) error {
	fs := flag.NewFlagSet("quota", flag.ContinueOnError)
	verbose := fs.Bool("verbose", false, "")
	if done, err := parseFlags(fs, args, quotaHelp); done || err != nil {
		return err
	}
	c, err := aext.New()
	if err != nil {
		return err
	}
	now := dates.Today()
	data, err := quota.FetchMonths(c, now)
	if err != nil {
		return err
	}
	fmt.Println(quota.Format(now, data, *verbose))
	return nil
}
