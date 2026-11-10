package main

import (
	"fmt"
	"os"

	"github.com/cloudwalksolutions/ccloud/cmd"
	"github.com/cloudwalksolutions/ccloud/internal/cloud/cloudwalk"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	release := fmt.Sprintf("%s (commit %s, built %s)", version, commit, date)
	if err := cmd.Run(os.Args[1:], os.Stdout, os.Stderr, cloudwalk.Cloud(), release); err != nil {
		fmt.Fprintln(os.Stderr, "ccloud:", err)
		os.Exit(1)
	}
}
