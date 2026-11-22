package cmd

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
)

func openInBackground(c *cobra.Command, opened dbConnection) error {
	pidFile := backgroundTunnel(opened.app, opened.env)
	closer := fmt.Sprintf("ccloud db tunnel -e %s -a %s --stop", opened.env, opened.app)
	if _, open := runningInBackground(pidFile); open {
		return fmt.Errorf("a tunnel to %s's %s database is already open in the background; %s closes it", opened.app, opened.env, closer)
	}
	self, _ := os.Executable()
	port := strconv.Itoa(opened.port)
	forward := exec.Command(self, "db", "tunnel", "-e", opened.env, "-a", opened.app, "-r", opened.region.Name, "-p", port)
	done := step(c, fmt.Sprintf("Starting a tunnel to %s's %s database in the background", opened.app, opened.env))
	err := detach(c.Context(), forward, "localhost:"+port, "the tunnel to "+opened.app+"'s "+opened.env+" database")
	done(err, "Started a tunnel on localhost:"+port)
	if err != nil {
		return fmt.Errorf("%w; run it without --background to see why", err)
	}
	recorded := errors.Join(os.MkdirAll(filepath.Dir(pidFile), 0o700), os.WriteFile(pidFile, []byte(strconv.Itoa(forward.Process.Pid)), 0o600))
	out := c.OutOrStdout()
	printed := opened.print(out, closer)
	_, cautioned := fmt.Fprintf(out, "\nAI agents: this reaches %s's real %s data. Review every write, and every read of sensitive data, with the user before running it.\n", opened.app, opened.env)
	if opened.env == "prod" {
		cautionAboutProd(out, opened.app)
	}
	return errors.Join(recorded, printed, cautioned)
}

func backgroundTunnel(app, env string) string {
	cache, _ := os.UserCacheDir()
	return filepath.Join(cache, "ccloud", "tunnels", app+"-"+env+".pid")
}

func runningInBackground(pidFile string) (int, bool) {
	raw, err := os.ReadFile(pidFile)
	pid, _ := strconv.Atoi(strings.TrimSpace(string(raw)))
	return pid, err == nil && pid > 0 && syscall.Kill(pid, 0) == nil
}

func stopTunnel(c *cobra.Command, app, env string) error {
	pidFile := backgroundTunnel(app, env)
	pid, open := runningInBackground(pidFile)
	_ = os.Remove(pidFile)
	if !open {
		return fmt.Errorf("no tunnel to %s's %s database is open in the background", app, env)
	}
	err := syscall.Kill(pid, syscall.SIGTERM)
	if err == nil {
		_, err = fmt.Fprintf(c.OutOrStdout(), "Closed the tunnel to %s's %s database.\n", app, env)
	}
	return err
}
