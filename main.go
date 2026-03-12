package main

import "github.com/team-xquare/xquare-cli/cmd"

// Set by GoReleaser via -ldflags
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	cmd.SetVersion(version, commit, date)
	cmd.Execute()
}
