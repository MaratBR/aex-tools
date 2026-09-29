// Package quota is the quota tool: AEXT hours quota for last and this month.
package quota

import (
	"flag"
	"fmt"

	"aex/internal/aext"
	"aex/internal/dates"
	quotadata "aex/internal/quota"
	"aex/internal/tool"
)

const quotaHelp = `Usage: quota [--verbose]

Shows AEXT hours quota for this month and last month: whether you are on track, working days
without hours, hours logged today, and hours/day needed for the rest of this month.

  --verbose  List leave warnings (unapproved or ignored leaves, leaves created by someone else)

Quota settings: HOURS_PER_DAY app setting (see configure), WorkingDaysCountry in internal/settings/config.go.`

// Tool is the quota tool.
var Tool = tool.Tool{Name: "quota", Summary: "Show AEXT hours quota for last and this month", Run: run}

func run(args []string) error {
	fs := flag.NewFlagSet("quota", flag.ContinueOnError)
	verbose := fs.Bool("verbose", false, "")
	if done, err := tool.ParseFlags(fs, args, quotaHelp); done || err != nil {
		return err
	}
	c, err := aext.New()
	if err != nil {
		return err
	}
	now := dates.Today()
	data, err := quotadata.FetchMonths(c, now)
	if err != nil {
		return err
	}
	fmt.Println(quotadata.Format(now, data, *verbose))
	return nil
}
