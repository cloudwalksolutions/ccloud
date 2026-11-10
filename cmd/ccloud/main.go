package main

import (
	"fmt"
	"os"

	"github.com/cloudwalksolutions/ccloud/cmd"
	"github.com/cloudwalksolutions/ccloud/internal/cloud/cloudwalk"
)

func main() {
	if err := cmd.Run(os.Args[1:], os.Stdout, os.Stderr, cloudwalk.Cloud()); err != nil {
		fmt.Fprintln(os.Stderr, "ccloud:", err)
		os.Exit(1)
	}
}
