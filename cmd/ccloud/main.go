package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/cloudwalksolutions/ccloud/cmd"
	"github.com/cloudwalksolutions/ccloud/internal/cloud/cloudwalk"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	release := fmt.Sprintf("%s (commit %s, built %s)", version, commit, date)
	err := cmd.RunContext(ctx, os.Args[1:], os.Stdout, os.Stderr, cloudwalk.Cloud(), release)
	stop()
	if err != nil {
		fmt.Fprintln(os.Stderr, "ccloud:", err)
		os.Exit(1)
	}
}
