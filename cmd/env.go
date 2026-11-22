package cmd

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"
	"k8s.io/client-go/rest"

	"github.com/cloudwalksolutions/ccloud/internal/cloud"
	"github.com/cloudwalksolutions/ccloud/internal/dotenv"
	"github.com/cloudwalksolutions/ccloud/internal/externalsecrets"
	"github.com/cloudwalksolutions/ccloud/internal/repo"
	"github.com/cloudwalksolutions/ccloud/internal/secretmanager"
)

func envCmd(home cloud.Cloud) *cobra.Command {
	env := &cobra.Command{
		Use:   "env",
		Short: "Your app's environment variables: set your machine up like its feature environments",
	}
	pull := &cobra.Command{
		Use:     "pull [--env <feature env>] [--force]",
		Short:   "Write the secrets a feature environment's API gets into your local env file",
		Example: "  ccloud env pull\n  ccloud env pull -e feat-new-checkout --force",
		Args:    cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			return envPull(c, home)
		},
	}
	pull.Flags().StringP("env", "e", "", "the feature environment to copy; the app's only one when left out")
	pull.Flags().StringP("region", "r", "", "the region running it; found for you when left out")
	pull.Flags().BoolP("force", "f", false, "replace values in your env file that differ from the environment's")
	pull.Flags().String("file", home.Env.File, "the env file to write, from the repo's root")
	env.AddCommand(pull)
	return env
}

func envPull(c *cobra.Command, home cloud.Cloud) error {
	wd, _ := os.Getwd()
	root, app, err := repo.App(wd, home.AppDir())
	if err == nil && app == "" {
		err = errors.New("run ccloud env pull inside the app's repo")
	}
	if err != nil {
		return err
	}
	env, _ := c.Flags().GetString("env")
	if slices.Contains(lasting, env) {
		return fmt.Errorf("env pull copies a feature environment, whose secrets come from the dev project; %s is not one", env)
	}
	env, region, err := featureEnv(c, home, app, env)
	if err != nil {
		return err
	}
	namespace, err := connect(region, func(cfg *rest.Config) (externalsecrets.Namespace, error) {
		return externalsecrets.In(cfg, env+"-"+app)
	})
	if err != nil {
		return err
	}
	secrets, err := namespace.Secrets(c.Context(), home.Env.Deployment, home.Env.Container)
	if errors.Is(err, externalsecrets.ErrNoDeployment) {
		return fmt.Errorf("%s's %s in %s runs no %s to copy", app, env, region.Name, home.Env.Deployment)
	}
	if err != nil {
		return err
	}
	vars, err := fetch(c, home, app, secrets)
	if err != nil {
		return err
	}
	file, _ := c.Flags().GetString("file")
	path := filepath.Join(root, file)
	existing, _ := os.ReadFile(path)
	force, _ := c.Flags().GetBool("force")
	merged, changes := dotenv.Merge(string(existing), vars, force)
	if len(changes.Differ) > 0 {
		differ := make([]string, len(changes.Differ))
		for i, d := range changes.Differ {
			differ[i] = fmt.Sprintf("  %s  %s → %s", d.Name, truncated(d.Was), truncated(d.Is))
		}
		return fmt.Errorf("%s differs from %s's %s; pass --force to replace:\n%s", file, app, env, strings.Join(differ, "\n"))
	}
	err = errors.Join(os.MkdirAll(filepath.Dir(path), 0o755), os.WriteFile(path, []byte(merged), 0o600))
	if err == nil {
		out := c.OutOrStdout()
		_, err = fmt.Fprint(out, report(paletteFor(out), changes, file, fmt.Sprintf("%s's %s in %s", app, env, region.Name)))
	}
	return err
}

func report(color palette, changes dotenv.Changes, file, source string) string {
	if len(changes.Added)+len(changes.Changed) == 0 {
		return file + " already matches " + source + ".\n"
	}
	lines := file + ", from " + source + ":\n"
	for _, name := range changes.Added {
		lines += "  " + color.paint("32", "added  ") + "  " + name + "\n"
	}
	for _, name := range changes.Changed {
		lines += "  " + color.paint("33", "changed") + "  " + name + "\n"
	}
	if changes.Unchanged > 0 {
		lines += fmt.Sprintf("%d unchanged.\n", changes.Unchanged)
	}
	return lines
}

var errNoGoogleSignIn = errors.New("no Google sign-in")

func fetch(c *cobra.Command, home cloud.Cloud, app string, secrets []externalsecrets.Secret) ([]dotenv.Var, error) {
	vars, err := read(c, home, app, secrets)
	if !errors.Is(err, errNoGoogleSignIn) {
		return vars, err
	}
	fmt.Fprintf(c.OutOrStdout(), "Signing you in to Google to read %s's secrets.\n", app)
	login := exec.CommandContext(c.Context(), "gcloud", "auth", "application-default", "login")
	login.Stdin, login.Stdout, login.Stderr = c.InOrStdin(), c.OutOrStdout(), c.ErrOrStderr()
	if login.Run() != nil {
		return nil, errors.New("signing in to Google failed, so nothing was pulled")
	}
	vars, err = read(c, home, app, secrets)
	if errors.Is(err, errNoGoogleSignIn) {
		return nil, fmt.Errorf("your Google sign-in was refused for %s's secrets, so nothing was pulled", app)
	}
	return vars, err
}

func read(c *cobra.Command, home cloud.Cloud, app string, secrets []externalsecrets.Secret) ([]dotenv.Var, error) {
	token, _ := exec.CommandContext(c.Context(), "gcloud", "auth", "application-default", "print-access-token").Output()
	manager := secretmanager.Client{URL: home.Env.SecretManager, Token: strings.TrimSpace(string(token))}
	vars := make([]dotenv.Var, len(secrets))
	var denied []string
	for i, secret := range secrets {
		var err error
		vars[i].Name = secret.Env
		vars[i].Value, err = manager.Access(c.Context(), secret.Project, secret.Name)
		if errors.Is(err, secretmanager.StatusError{Code: http.StatusUnauthorized}) {
			return nil, errNoGoogleSignIn
		}
		if errors.Is(err, secretmanager.StatusError{Code: http.StatusForbidden}) {
			denied = append(denied, secret.Name)
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("reading %s from %s: %w", secret.Name, secret.Project, err)
		}
	}
	if len(denied) > 0 {
		return nil, fmt.Errorf("you may not read %s's %s in %s; ask to be added to %s's developers", app, and(denied), secrets[0].Project, app)
	}
	return vars, nil
}

func and(names []string) string {
	last := len(names) - 1
	if last == 0 {
		return names[0]
	}
	return strings.Join(names[:last], ", ") + " and " + names[last]
}

var lasting = []string{"prod", "stage", "ci"}

func featureEnv(c *cobra.Command, home cloud.Cloud, app, env string) (string, cloud.Region, error) {
	if env != "" {
		name, _ := c.Flags().GetString("region")
		region, err := regionFor(c, home, name, app, env)
		return env, region, err
	}
	environments, err := everyRegionsEnvironments(c, home, app)
	if err != nil {
		return "", cloud.Region{}, err
	}
	var features, hosts []string
	for _, region := range home.Regions {
		for _, environment := range environments[region.Name] {
			if !slices.Contains(lasting, environment.Name) {
				features = append(features, environment.Name)
				hosts = append(hosts, region.Name)
			}
		}
	}
	switch {
	case len(features) == 0:
		return "", cloud.Region{}, fmt.Errorf("no feature environment of %s is running to copy; open a pull request, or name one with -e", app)
	case len(features) > 1:
		return "", cloud.Region{}, fmt.Errorf("%s has %d feature environments running, %s; name the one to copy with -e", app, len(features), and(features))
	}
	region, err := home.Find(hosts[0])
	return features[0], region, err
}
