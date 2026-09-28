package main

import (
	"flag"
	"fmt"

	"aex/internal/aext"
	"aex/internal/dates"
	"aex/internal/quota"
)

const quotaHelp = `Usage: quota

Shows AEXT hours quota for last month and this month: working days, expected vs logged
hours, % filled, hours behind, and hours/day needed for the rest of this month.
Quota settings: HOURS_PER_DAY app setting (see configure), WorkingDaysCountry in internal/settings/config.go.`

func quotaTool(args []string) error {
	if done, err := parseFlags(flag.NewFlagSet("quota", flag.ContinueOnError), args, quotaHelp); done || err != nil {
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
	fmt.Println(quota.Format(now, data))
	return nil
}
