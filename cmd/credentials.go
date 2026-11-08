package cmd

import (
	"fmt"
	"net"
	"os/exec"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"ccloud/internal/regions"
)

func credentials(c *cobra.Command, region regions.Region) error {
	if err := tunnel(region); err != nil {
		return err
	}
	return nil
}

func tunnel(region regions.Region) error {
	address := fmt.Sprintf("127.0.0.1:%d", region.Port)
	if listening(address) {
		return nil
	}
	cloudflared := exec.Command("cloudflared", "access", "tcp", "--hostname", region.Host, "--url", address)
	cloudflared.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cloudflared.Start(); err != nil {
		return fmt.Errorf("starting cloudflared for %s: %w", region.Host, err)
	}
	exited := make(chan error, 1)
	go func() { exited <- cloudflared.Wait() }()

	deadline := time.After(20 * time.Second)
	for {
		select {
		case err := <-exited:
			return fmt.Errorf("cloudflared exited before %s was reachable at %s: %v", region.Host, address, err)
		case <-deadline:
			return fmt.Errorf("cloudflared did not open %s within 20s for %s", address, region.Host)
		case <-time.After(100 * time.Millisecond):
			if listening(address) {
				return nil
			}
		}
	}
}

func listening(address string) bool {
	conn, err := net.DialTimeout("tcp", address, 200*time.Millisecond)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}
