// cm-release: plugin tool (see internal/plugin), built to plugins\cm-release.exe next to aex.exe.
// A group of CM release tools, each in its own file:
//   - pull-all (pull.go): pulls master, PROD and staging in every CM repo.
//   - prepare-release (prepare.go): creates and pushes release/VERSION off staging in every CM repo.
//   - merge-prod (merge.go): merges release/VERSION into PROD and tags it in every CM repo.
//   - jira-handoff (handoff.go): hands an epic's "Ready for Production" tickets over to QA.
//
// The git tools share git.go.
package main

import (
	"aex/internal/plugin"
	"aex/internal/tool"
)

//go:generate go run github.com/tc-hib/go-winres@v0.3.3 make --in winres/winres.json --out rsrc --arch amd64,arm64

func main() {
	plugin.Main(tool.Tool{
		Name:    "cm-release",
		Summary: "CM release tools",
		Sub: []tool.Tool{
			{Name: "pull-all", Summary: "Pull master, PROD and staging in every CM repo", Run: runPull},
			{Name: "prepare-release", Summary: "Create release/VERSION off staging in every CM repo, review diffs, push", Run: runPrepare},
			{Name: "merge-prod", Summary: "Merge release/VERSION into PROD in every CM repo, tag vVERSION, push", Run: runMerge},
			{Name: "jira-handoff", Summary: "Hand off an epic's Ready for Production tickets to QA", Run: runHandoff},
		},
	}, plugin.Jira)
}
