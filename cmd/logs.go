package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	clientauthentication "k8s.io/client-go/pkg/apis/clientauthentication/v1"

	"github.com/cloudwalksolutions/ccloud/internal/cloud"
	"github.com/cloudwalksolutions/ccloud/internal/logs"
)

func logsCmd(home cloud.Cloud) *cobra.Command {
	command := appCmd(home, &cobra.Command{
		Use:     "logs --app <app> --env <env> --region <region>",
		Short:   "Read an app's logs in one environment, as far back as they are kept",
		Example: "  ccloud logs -a <app> -e prod -r " + home.Regions[0].Name + " --level error --since 6h",
	}, func(c *cobra.Command, region cloud.Region, app string) error {
		return readLogs(c, home, region, app)
	})
	flags := command.Flags()
	flags.StringP("env", "e", "", "the environment: prod, stage, ci, or a feature environment's name")
	flags.Duration("since", time.Hour, "how far back to read")
	flags.Int("tail", 200, "at most this many lines, the newest")
	flags.String("pod", "", "only pods whose name starts with this")
	flags.StringP("container", "c", "", "only this container")
	flags.StringP("grep", "g", "", "only lines containing this text")
	flags.StringP("regex", "E", "", "only lines matching this regular expression")
	flags.StringP("level", "l", "", "only errors (error) or errors and warnings (warn)")
	flags.BoolP("follow", "f", false, "keep reading new lines until you press Ctrl-C")
	flags.StringP("output", "o", "pretty", "pretty, raw (each line as stored) or json (one entry per line)")
	flags.DurationP("interval", "i", 2*time.Second, "with --follow, how often to look for new lines")
	return command
}

func readLogs(c *cobra.Command, home cloud.Cloud, region cloud.Region, app string) error {
	env, _ := c.Flags().GetString("env")
	if env == "" {
		return fmt.Errorf("which environment? ccloud logs --env <env>, such as prod, stage, ci or a feature environment's name")
	}
	gateway, err := signInToLogs(c, home, region)
	if err != nil {
		return err
	}
	query := logsQuery(c, env+"-"+app)
	follow, _ := c.Flags().GetBool("follow")
	interval, _ := c.Flags().GetDuration("interval")
	format, _ := c.Flags().GetString("output")
	printer := logPrinter{out: c.OutOrStdout(), format: format, colour: paletteFor(c.OutOrStdout())}
	ctx := c.Context()
	for {
		entries, err := gateway.Read(ctx, query)
		if ctx.Err() != nil {
			return nil
		}
		var refused logs.Refused
		if errors.As(err, &refused) && refused.Status == http.StatusForbidden {
			return fmt.Errorf("you may not read %s's %s logs in %s: %s", app, env, region.Name, refused.Reason)
		}
		if err != nil {
			return err
		}
		for _, entry := range entries {
			printer.print(entry)
		}
		if !follow {
			return nil
		}
		if len(entries) > 0 {
			query.Set("start", entries[len(entries)-1].Time.Add(time.Nanosecond).UTC().Format(time.RFC3339Nano))
		}
		query.Set("direction", "forward")
		select {
		case <-ctx.Done():
		case <-time.After(interval):
		}
	}
}

func logsQuery(c *cobra.Command, namespace string) url.Values {
	flags := c.Flags()
	since, _ := flags.GetDuration("since")
	tail, _ := flags.GetInt("tail")
	query := url.Values{
		"namespace": {namespace},
		"start":     {time.Now().Add(-since).UTC().Format(time.RFC3339)},
		"limit":     {strconv.Itoa(tail)},
		"direction": {"backward"},
	}
	for flag, param := range map[string]string{"pod": "pod", "container": "container", "grep": "contains", "regex": "regex", "level": "level"} {
		if value, _ := flags.GetString(flag); value != "" {
			query.Set(param, value)
		}
	}
	return query
}

func signInToLogs(c *cobra.Command, home cloud.Cloud, region cloud.Region) (logs.Gateway, error) {
	access, err := accessToken(c, region)
	if err != nil {
		return logs.Gateway{}, err
	}
	credential, err := exec.Command("kubectl-oidc_login", oidcArgs(home)...).Output()
	if err != nil {
		return logs.Gateway{}, err
	}
	var exchanged clientauthentication.ExecCredential
	if err := json.Unmarshal(credential, &exchanged); err != nil || exchanged.Status == nil {
		return logs.Gateway{}, fmt.Errorf("kubelogin gave no token: %s", credential)
	}
	return logs.Gateway{URL: region.Logs, AccessToken: access, IDToken: exchanged.Status.Token}, nil
}

func accessToken(c *cobra.Command, region cloud.Region) (string, error) {
	token, err := exec.Command("cloudflared", "access", "token", "--app="+region.Logs).Output()
	if err == nil {
		return strings.TrimSpace(string(token)), nil
	}
	login := exec.Command("cloudflared", "access", "login", region.Logs)
	login.Stdout, login.Stderr = c.ErrOrStderr(), c.ErrOrStderr()
	if err := login.Run(); err != nil {
		return "", fmt.Errorf("signing in to %s through Cloudflare Access: %w", region.Logs, err)
	}
	token, err = exec.Command("cloudflared", "access", "token", "--app="+region.Logs).Output()
	return strings.TrimSpace(string(token)), err
}
