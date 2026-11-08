package main

import (
	"fmt"
	"os"

	"ccloud/cmd"
)

func main() {
	if err := cmd.Run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "ccloud:", err)
		os.Exit(1)
	}
}
