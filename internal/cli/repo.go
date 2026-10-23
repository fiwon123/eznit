package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/fiwon123/eznit/internal/gitprovider"
	"github.com/fiwon123/eznit/pkg/helper"
)

// The environment the repo commands read:
//
//   - EZNIT_GITHUB_API overrides the GitHub API address, for a GitHub
//     Enterprise instance or a test.
//   - EZNIT_REPOS is the folder a clone lands under. The default is
//     `<home>/eznit`: the folder belongs to eznit, not to the directory the
//     user happened to run the command from.
const (
	githubAPIEnv = "EZNIT_GITHUB_API"
	reposEnv     = "EZNIT_REPOS"

	defaultProvider = "github"
)

// gitBinary is the git the clone runs. It is a variable so a test can put a
// fake in its place, and so the error message can name what is missing.
var gitBinary = "git"

// RepoCmd works with git hosts: store a token, list what it can reach, clone
// one repository.
type RepoCmd struct {
	Auth  RepoAuthCmd  `cmd:"" help:"store the token of a git host"`
	List  RepoListCmd  `cmd:"" help:"list the repositories the token can reach"`
	Clone RepoCloneCmd `cmd:"" help:"clone a repository into an eznit folder"`
}

// providerTokenKey is where a provider's token lives in the config file. One
// key per provider, so storing a second one does not replace the first.
func providerTokenKey(provider string) string {
	return provider + "_token"
}

// providerFor returns the provider by name. GitHub is the only one today; a
// second host is a second case here and a second implementation of the
// interface, not a second set of commands.
func providerFor(name string) (gitprovider.Provider, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "github":
		return gitprovider.NewGitHub(os.Getenv(githubAPIEnv)), nil
	default:
		return nil, fmt.Errorf("unknown git host %q — github is the one that is supported", name)
	}
}

// providerToken reads the token of one provider. A missing token is not a
// failure of the command: it tells the user what to run.
func providerToken(provider string) (string, bool) {
	token, err := configValue(providerTokenKey(provider))
	if err != nil {
		return "", false
	}

	return token, true
}

// RepoAuthCmd stores the personal access token of a git host.
type RepoAuthCmd struct {
	Provider string `help:"the git host" default:"github"`
	// A token on the command line is visible in the process list for as
	// long as the command runs, so the prompt and --token-file are the
	// quieter paths. The flag is there because scripts need one.
	Token     string `help:"the personal access token (prefer the prompt or --token-file)" optional:""`
	TokenFile string `help:"read the token from a file" type:"path" optional:""`
	Remove    bool   `help:"forget the stored token"`
}

// Run stores or removes the provider token.
func (cmd *RepoAuthCmd) Run(g *Globals) error {
	provider, err := providerFor(cmd.Provider)
	if err != nil {
		g.logger.Warn(err.Error())
		return nil
	}

	key := providerTokenKey(provider.Name())

	if cmd.Remove {
		if err := removeConfigValue(key); err != nil {
			g.logger.Warn("failed to remove the token. ", slog.String("error", err.Error()))
			return nil
		}

		fmt.Printf("\nremoved the %s token\n", provider.Name())
		return nil
	}

	token, ok := readToken(cmd, g)
	if !ok {
		return nil
	}

	// Confirm the token works before it is stored: a typo is found now, not
	// on the next clone. A token the host refuses is not stored; a host that
	// cannot be reached is a warning, because the token may still be good.
	account, err := provider.Viewer(context.Background(), token)
	switch {
	case err == nil:
	case errors.Is(err, gitprovider.ErrUnauthorized):
		g.logger.Warn("the host refused the token — check that it is valid and can read repositories")
		return nil
	default:
		g.logger.Warn("the token could not be verified. ", slog.String("error", err.Error()))
	}

	if err := setConfigValue(key, token); err != nil {
		g.logger.Warn("failed to store the token. ", slog.String("error", err.Error()))
		return nil
	}

	g.logger.Debug("stored a provider token", slog.String("token_fp", helper.Fingerprint(token)))

	if account != "" {
		fmt.Printf("\nstored a %s token for %s\n", provider.Name(), account)
	} else {
		fmt.Printf("\nstored a %s token\n", provider.Name())
	}

	return nil
}

// readToken is the token from the flag, the file, or the prompt — in that
// order. The prompt does not echo, so a token typed at a terminal is not left
// on the screen.
func readToken(cmd *RepoAuthCmd, g *Globals) (string, bool) {
	if cmd.Token != "" {
		if cmd.TokenFile != "" {
			g.logger.Warn("--token and --token-file are two ways to give one token; pick one")
			return "", false
		}

		return strings.TrimSpace(cmd.Token), true
	}

	if cmd.TokenFile != "" {
		raw, err := os.ReadFile(cmd.TokenFile)
		if err != nil {
			g.logger.Warn("failed to read the token file. ", slog.String("error", err.Error()))
			return "", false
		}

		token := strings.TrimSpace(string(raw))
		if token == "" {
			g.logger.Warn("the token file is empty. ", slog.String("path", cmd.TokenFile))
			return "", false
		}

		return token, true
	}

	token, err := promptPassword("Enter the personal access token: ")
	if err != nil {
		g.logger.Warn("failed to read the token. ", slog.String("error", err.Error()))
		return "", false
	}

	token = strings.TrimSpace(token)
	if token == "" {
		g.logger.Warn("a token is required. ")
		return "", false
	}

	return token, true
}

// RepoListCmd lists the repositories a token can reach.
type RepoListCmd struct {
	Provider string `help:"the git host" default:"github"`
}

// Run lists the repositories.
func (cmd *RepoListCmd) Run(g *Globals) error {
	provider, err := providerFor(cmd.Provider)
	if err != nil {
		g.logger.Warn(err.Error())
		return nil
	}

	token, ok := providerToken(provider.Name())
	if !ok {
		g.logger.Warn("no " + provider.Name() + " token — run `eznit repo auth` first")
		return nil
	}

	repositories, err := provider.ListRepositories(context.Background(), token)
	if err != nil {
		g.logger.Warn("the repositories could not be listed. ", slog.String("error", err.Error()))
		return nil
	}

	if len(repositories) == 0 {
		fmt.Printf("\nno repositories the %s token can reach\n", provider.Name())
		return nil
	}

	fmt.Println()

	for i, repository := range repositories {
		visibility := "public"
		if repository.Private {
			visibility = "private"
		}

		fmt.Printf("%3d. %s (%s, %s)\n", i+1, repository.FullName, visibility, repository.DefaultBranch)
	}

	fmt.Println()

	return nil
}

// RepoCloneCmd clones one repository.
type RepoCloneCmd struct {
	Name     string `arg:"" help:"the repository as owner/name"`
	Provider string `help:"the git host" default:"github"`
	Dir      string `help:"the folder to clone under (default $EZNIT_REPOS or ~/eznit)" type:"path" optional:""`
}

// Run clones the repository into the eznit folder.
func (cmd *RepoCloneCmd) Run(g *Globals) error {
	provider, err := providerFor(cmd.Provider)
	if err != nil {
		g.logger.Warn(err.Error())
		return nil
	}

	// The name is checked before it is used, as it is used to build a path:
	// a name the host would never hand back does not reach the filesystem.
	fullName := strings.TrimSpace(cmd.Name)
	if !gitprovider.ValidFullName(fullName) {
		g.logger.Warn("a repository is written as owner/name. ", slog.String("repository", fullName))
		return nil
	}

	token, ok := providerToken(provider.Name())
	if !ok {
		g.logger.Warn("no " + provider.Name() + " token — run `eznit repo auth` first")
		return nil
	}

	repository, err := provider.GetRepository(context.Background(), token, fullName)
	if err != nil {
		if errors.Is(err, gitprovider.ErrNotFound) {
			g.logger.Warn("no such repository, or the token cannot see it. ", slog.String("repository", fullName))
			return nil
		}

		g.logger.Warn("the repository could not be read. ", slog.String("error", err.Error()))
		return nil
	}

	if repository.CloneURL == "" {
		g.logger.Warn("the host did not give a clone URL. ", slog.String("repository", fullName))
		return nil
	}

	root, ok := reposRoot(cmd.Dir, g)
	if !ok {
		return nil
	}

	target, ok := cloneTarget(root, provider.Name(), repository.FullName, g)
	if !ok {
		return nil
	}

	if _, err := exec.LookPath(gitBinary); err != nil {
		g.logger.Warn("git is not installed, or not on PATH")
		return nil
	}

	headerKey, headerValue := provider.GitAuthHeader(token)

	fmt.Printf("\ncloning %s into %s\n", repository.FullName, target)

	if err := gitClone(context.Background(), repository.CloneURL, target, headerKey, headerValue, os.Stdout, os.Stderr); err != nil {
		g.logger.Warn("the clone failed. ", slog.String("error", err.Error()))
		return nil
	}

	fmt.Printf("\ncloned %s\n", repository.FullName)

	return nil
}

// reposRoot is where clones land: --dir, then EZNIT_REPOS, then `<home>/eznit`.
func reposRoot(dir string, g *Globals) (string, bool) {
	if dir != "" {
		return dir, true
	}

	if fromEnv := strings.TrimSpace(os.Getenv(reposEnv)); fromEnv != "" {
		return fromEnv, true
	}

	home, err := os.UserHomeDir()
	if err != nil {
		g.logger.Warn("the home folder is not known; pass --dir. ", slog.String("error", err.Error()))
		return "", false
	}

	return filepath.Join(home, "eznit"), true
}

// cloneTarget is the folder one repository is cloned into:
// `<root>/<provider>/<owner>/<name>`. An existing folder is only used when it
// is empty — a clone that finds a file would otherwise mix two trees.
func cloneTarget(root string, provider string, fullName string, g *Globals) (string, bool) {
	if !gitprovider.ValidFullName(fullName) {
		g.logger.Warn("the host gave a repository name that cannot be a path. ", slog.String("repository", fullName))
		return "", false
	}

	target := filepath.Join(root, provider, filepath.FromSlash(fullName))

	info, err := os.Stat(target)
	if err != nil {
		if os.IsNotExist(err) {
			return target, true
		}

		g.logger.Warn("the clone folder could not be read. ", slog.String("error", err.Error()))
		return "", false
	}

	if !info.IsDir() {
		g.logger.Warn("the clone target is a file, not a folder. ", slog.String("path", target))
		return "", false
	}

	entries, err := os.ReadDir(target)
	if err != nil {
		g.logger.Warn("the clone folder could not be read. ", slog.String("error", err.Error()))
		return "", false
	}

	if len(entries) > 0 {
		g.logger.Warn("the clone folder is not empty. ", slog.String("path", target))
		return "", false
	}

	return target, true
}

// cloneArgs is the git command line. The URL is the plain HTTPS one the host
// gave: no token in the URL, so it is not in argv and not in `.git/config`.
func cloneArgs(repoURL string, target string) []string {
	return []string{"clone", "--", repoURL, target}
}

// cloneEnv is the environment the clone runs with: the authentication goes in
// through git's config environment, and anything the caller's environment
// already had for the same keys is dropped first so one value cannot shadow
// the other.
func cloneEnv(headerKey string, headerValue string, base []string) []string {
	env := make([]string, 0, len(base)+4)

	for _, entry := range base {
		if strings.HasPrefix(entry, "GIT_CONFIG_COUNT=") ||
			strings.HasPrefix(entry, "GIT_CONFIG_KEY_") ||
			strings.HasPrefix(entry, "GIT_CONFIG_VALUE_") ||
			strings.HasPrefix(entry, "GIT_TERMINAL_PROMPT=") {
			continue
		}

		env = append(env, entry)
	}

	return append(env,
		"GIT_TERMINAL_PROMPT=0",
		"GIT_CONFIG_COUNT=1",
		"GIT_CONFIG_KEY_0="+headerKey,
		"GIT_CONFIG_VALUE_0="+headerValue,
	)
}

// gitClone runs git. The token reaches git through the environment only; it
// is not an argument, so it does not appear in the process list, and git
// never writes it into the clone's remote.
func gitClone(ctx context.Context, repoURL string, target string, headerKey string, headerValue string, stdout io.Writer, stderr io.Writer) error {
	command := exec.CommandContext(ctx, gitBinary, cloneArgs(repoURL, target)...)
	command.Env = cloneEnv(headerKey, headerValue, os.Environ())
	command.Stdout = stdout
	command.Stderr = stderr

	if err := command.Run(); err != nil {
		return fmt.Errorf("git clone failed: %w", err)
	}

	return nil
}
