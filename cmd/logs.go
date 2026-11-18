package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	clientauthentication "k8s.io/client-go/pkg/apis/clientauthentication/v1"

	"github.com/cloudwalksolutions/ccloud/internal/cloud"
	"github.com/cloudwalksolutions/ccloud/internal/gateway"
)

func logsCmd(home cloud.Cloud) *cobra.Command {
	command := appCmd(home, &cobra.Command{
		Use:     "logs --env <env> [--app <app>]",
		Short:   "Read an app's logs in one environment, as far back as they are kept",
		Example: "  ccloud logs -e prod --level error --since 6h",
	}, namedEnv, func(c *cobra.Command, region cloud.Region, app string) error {
		return readLogs(c, home, region, app)
	})
	flags := command.Flags()
	flags.StringP("env", "e", "", "the environment: prod, stage, ci, or a feature environment's name")
	flags.Duration("since", time.Hour, "how far back to read")
	flags.String("since-time", "", "read from this time, such as 2026-10-26T07:00:00Z, rather than --since ago")
	flags.String("until", "", "read up to this time, or a while ago such as 10m; now when left out")
	flags.Int("tail", 200, "at most this many lines, the newest")
	flags.String("pod", "", "only pods whose name starts with this")
	flags.StringArrayP("container", "c", nil, "only this container; repeat for several")
	flags.StringP("grep", "g", "", "only lines containing this text")
	flags.StringP("regex", "E", "", "only lines matching this regular expression")
	flags.StringP("level", "l", "", "only errors (error) or errors and warnings (warn)")
	flags.BoolP("follow", "f", false, "keep reading new lines until you press Ctrl-C")
	flags.Bool("fail-on-match", false, "exit non-zero when any line matched, such as with --level error in a script")
	flags.StringP("output", "o", "pretty", "pretty, raw (each line as stored) or json (one entry per line)")
	command.AddCommand(logContainersCmd(home))
	return command
}

func logContainersCmd(home cloud.Cloud) *cobra.Command {
	command := appCmd(home, &cobra.Command{
		Use:     "containers --env <env> [--app <app>]",
		Short:   "List the containers an environment has logged in the last day, to pick from with -c",
		Example: "  ccloud logs containers -e stage",
	}, namedEnv, func(c *cobra.Command, region cloud.Region, app string) error {
		env, _ := c.Flags().GetString("env")
		signedIn, err := signInToGateway(c, home, region)
		var containers []string
		if err == nil {
			containers, err = signedIn.Containers(c.Context(), url.Values{
				"namespace": {env + "-" + app},
				"start":     {time.Now().Add(-24 * time.Hour).UTC().Format(time.RFC3339)},
			})
		}
		for _, container := range containers {
			fmt.Fprintln(c.OutOrStdout(), container)
		}
		return err
	})
	command.Flags().StringP("env", "e", "", "the environment: prod, stage, ci, or a feature environment's name")
	return command
}

func namedEnv(c *cobra.Command) (string, error) {
	env, _ := c.Flags().GetString("env")
	if env == "" {
		return "", fmt.Errorf("which environment? %s --env <env>, such as prod, stage, ci or a feature environment's name", c.CommandPath())
	}
	return env, nil
}

func readLogs(c *cobra.Command, home cloud.Cloud, region cloud.Region, app string) error {
	env, _ := c.Flags().GetString("env")
	reader := &logReader{c: c, home: home, region: region}
	query, err := logsQuery(c, env+"-"+app)
	if err != nil {
		return err
	}
	follow, _ := c.Flags().GetBool("follow")
	strict, _ := c.Flags().GetBool("fail-on-match")
	format, _ := c.Flags().GetString("output")
	if !slices.Contains([]string{"pretty", "raw", "json"}, format) {
		return fmt.Errorf("output is pretty, raw or json, not %q", format)
	}
	printer := logPrinter{out: c.OutOrStdout(), format: format, color: paletteFor(c.OutOrStdout())}
	page, err := reader.read(query)
	if c.Context().Err() != nil {
		return nil
	}
	if err != nil {
		return mayNotRead(err, app, env, region)
	}
	entries := page.Entries
	for _, entry := range entries {
		printer.print(entry)
	}
	switch {
	case strict && len(entries) > 0:
		return fmt.Errorf("%s matched", quantity(len(entries), "line"))
	case follow:
		from := time.Now().UTC()
		if len(entries) > 0 {
			from = entries[len(entries)-1].Time
		}
		return followLogs(c, reader, query, printer, from, strict, app, env, region)
	}
	if format == "json" && len(entries) > 0 {
		_ = json.NewEncoder(c.OutOrStdout()).Encode(map[string]string{"next": entries[len(entries)-1].Time.Add(time.Nanosecond).UTC().Format(time.RFC3339Nano)})
	}
	if page.Truncated {
		fmt.Fprintf(c.ErrOrStderr(), "Showed the newest %d lines; raise --tail or narrow --since or --until for more.\n", len(entries))
	}
	return nil
}

var errMatched = errors.New("a line matched")

func followLogs(c *cobra.Command, reader *logReader, query url.Values, printer logPrinter, from time.Time, strict bool, app, env string, region cloud.Region) error {
	for _, key := range []string{"limit", "direction", "end"} {
		query.Del(key)
	}
	printed := map[gateway.Entry]bool{}
	wait, signedInAgain := time.Second, false
	for {
		query.Set("start", from.UTC().Format(time.RFC3339Nano))
		end, err := reader.stream(query, func(entry gateway.Entry) error {
			if printed[entry] {
				return nil
			}
			if entry.Time.After(from) {
				from, printed = entry.Time, map[gateway.Entry]bool{}
			}
			printed[entry] = true
			printer.print(entry)
			wait = time.Second
			if strict {
				return errMatched
			}
			return nil
		})
		var refused gateway.Refused
		errors.As(err, &refused)
		switch {
		case c.Context().Err() != nil:
			return nil
		case errors.Is(err, errMatched), errors.Is(err, gateway.ErrNotLogs), reader.signedIn == nil:
			return err
		case end == "max duration":
			continue
		case end == "access revoked":
			return fmt.Errorf("you may no longer read %s's %s logs in %s", app, env, region.Name)
		case refused.Status == http.StatusUnauthorized && !signedInAgain:
			reader.signedIn, signedInAgain = nil, true
			continue
		case refused.Status >= 400 && refused.Status < 500:
			return mayNotRead(err, app, env, region)
		}
		select {
		case <-c.Context().Done():
			return nil
		case <-time.After(wait):
		}
		wait = min(2*wait, 30*time.Second)
	}
}

func mayNotRead(err error, app, env string, region cloud.Region) error {
	var refused gateway.Refused
	if errors.As(err, &refused) && refused.Status == http.StatusForbidden {
		err = fmt.Errorf("you may not read %s's %s logs in %s: %s", app, env, region.Name, refused.Reason)
	}
	return err
}

func logsQuery(c *cobra.Command, namespace string) (url.Values, error) {
	flags := c.Flags()
	since, _ := flags.GetDuration("since")
	tail, _ := flags.GetInt("tail")
	query := url.Values{
		"namespace": {namespace},
		"start":     {time.Now().Add(-since).UTC().Format(time.RFC3339)},
		"limit":     {strconv.Itoa(tail)},
		"direction": {"backward"},
	}
	if start, _ := flags.GetString("since-time"); start != "" {
		query.Set("start", start)
	}
	if until, _ := flags.GetString("until"); until != "" {
		end, err := endOf(until)
		if err != nil {
			return nil, err
		}
		query.Set("end", end)
	}
	for flag, param := range map[string]string{"pod": "pod", "grep": "contains", "regex": "regex", "level": "level"} {
		if value, _ := flags.GetString(flag); value != "" {
			query.Set(param, value)
		}
	}
	if containers, _ := flags.GetStringArray("container"); len(containers) > 0 {
		query["container"] = containers
	}
	return query, nil
}

func endOf(until string) (string, error) {
	if _, err := time.Parse(time.RFC3339Nano, until); err == nil {
		return until, nil
	}
	ago, err := time.ParseDuration(until)
	if err != nil {
		return "", fmt.Errorf("--until is a time like 2026-10-26T08:00:00Z or a while ago like 10m, not %q", until)
	}
	return time.Now().Add(-ago).UTC().Format(time.RFC3339), nil
}

type logReader struct {
	c        *cobra.Command
	home     cloud.Cloud
	region   cloud.Region
	signedIn *gateway.Gateway
}

func (r *logReader) read(query url.Values) (gateway.Page, error) {
	if err := r.signIn(); err != nil {
		return gateway.Page{}, err
	}
	return r.signedIn.Read(r.c.Context(), query)
}

func (r *logReader) stream(query url.Values, each func(gateway.Entry) error) (string, error) {
	if err := r.signIn(); err != nil {
		return "", err
	}
	return r.signedIn.Stream(r.c.Context(), query, each)
}

func (r *logReader) signIn() error {
	if r.signedIn != nil {
		return nil
	}
	g, err := signInToGateway(r.c, r.home, r.region)
	r.signedIn = &g
	if err != nil {
		r.signedIn = nil
	}
	return err
}

func signInToGateway(c *cobra.Command, home cloud.Cloud, region cloud.Region) (gateway.Gateway, error) {
	address, err := url.Parse(region.Gateway)
	if err != nil {
		return gateway.Gateway{}, fmt.Errorf("%s's gateway %q is not a URL: %w", region.Name, region.Gateway, err)
	}
	access, err := accessToken(c, region)
	if err != nil {
		return gateway.Gateway{}, err
	}
	oidc := exec.CommandContext(c.Context(), "kubectl-oidc_login", oidcArgs(home)...)
	oidc.Stderr = c.ErrOrStderr()
	credential, err := oidc.Output()
	var exchanged clientauthentication.ExecCredential
	if err != nil || json.Unmarshal(credential, &exchanged) != nil || exchanged.Status == nil {
		return gateway.Gateway{}, fmt.Errorf("signing in to %s failed", home.Issuer)
	}
	return gateway.Gateway{URL: address, AccessToken: access, IDToken: exchanged.Status.Token}, nil
}

func accessToken(c *cobra.Command, region cloud.Region) (string, error) {
	token, err := exec.CommandContext(c.Context(), "cloudflared", "access", "token", "--app="+region.Gateway).Output()
	if err == nil {
		return strings.TrimSpace(string(token)), nil
	}
	login := exec.CommandContext(c.Context(), "cloudflared", "access", "login", "--quiet", region.Gateway)
	login.Stdout, login.Stderr = c.ErrOrStderr(), c.ErrOrStderr()
	if err := login.Run(); err != nil {
		return "", fmt.Errorf("signing in to %s through Cloudflare Access: %w", region.Gateway, err)
	}
	token, err = exec.CommandContext(c.Context(), "cloudflared", "access", "token", "--app="+region.Gateway).Output()
	return strings.TrimSpace(string(token)), err
}
