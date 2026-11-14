package serverctl

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// The database step's failures. Each one is a different sentence for a person,
// so the panel maps them rather than showing a command's exit status.
var (
	// ErrNoCompose is a folder with no compose file: the file the database and
	// the migrations are started from.
	ErrNoCompose = errors.New("this folder has no compose file")

	// ErrNoRuntime is a machine with no container runtime on PATH.
	ErrNoRuntime = errors.New("no docker or podman on PATH")

	// ErrNoPassword is a folder without its database secret. Postgres applies
	// its password when it creates its volume, so one cannot be invented later.
	ErrNoPassword = errors.New("no " + SecretPath + " in this folder")

	// ErrNoAnswer is a database that was started and still does not answer.
	ErrNoAnswer = errors.New("nothing answers on the database address")
)

// The defaults the compose file uses when .env says nothing. They are repeated
// here because the probe has to look somewhere before it can be told otherwise.
const (
	defaultDBHost = "127.0.0.1"
	defaultDBPort = "5432"
)

// ComposeFileName is the name a release bundle uses. Compose also accepts the
// three below, and a folder holding one of those is startable all the same.
const ComposeFileName = "docker-compose.yml"

// Result is what the database step did.
type Result struct {
	// Started is true when nothing answered and Compose brought the database up.
	Started bool
	// Migrated is true when the folder's migration job ran.
	Migrated bool
}

// Prepare makes the folder's database usable before the API starts.
//
// The rule the panel relies on: it returns nil only when the database answers at
// the address the API will use. Anything else is an error the caller can name,
// because starting an API with no database produces a log of pq errors and a
// green health dot — the state this step exists to prevent.
//
// ctx bounds the container runtime. A daemon that never answers must not leave
// the window with a modal it cannot dismiss, so a cancel or the deadline ends
// the child process. out receives the runtime's output as it arrives.
func Prepare(ctx context.Context, dir string, out io.Writer) (Result, error) {
	return preparer{
		ctx:   ctx,
		dir:   dir,
		out:   out,
		probe: 750 * time.Millisecond,
		wait:  time.Minute,
		poll:  250 * time.Millisecond,
		reach: reachable,
		look:  exec.LookPath,
	}.run()
}

// preparer is Prepare with its clock and its seams visible, so a test can drive
// every branch without a database or a container runtime.
type preparer struct {
	ctx   context.Context
	dir   string
	out   io.Writer
	probe time.Duration
	wait  time.Duration
	poll  time.Duration
	reach func(host, port string, timeout time.Duration) bool
	look  func(string) (string, error)
}

func (p preparer) run() (Result, error) {
	if p.ctx == nil {
		p.ctx = context.Background()
	}

	// The .env is where the address comes from, and a first start creates it
	// from the example — the same call the supervisor makes, so both agree.
	env, err := LoadOrCreate(p.dir)
	if err != nil {
		return Result{}, err
	}

	address := databaseAddress(env)

	// The runtime inherits this process's environment (a container runtime
	// needs PATH and HOME), and then the folder's values, so the file wins over
	// whatever the shell exported. Compose reads the environment for its
	// `${...}` substitutions, and the probe must look at the same database the
	// containers will use.
	environ := append(os.Environ(), env.ChildEnv()...)

	composeFile := p.composeFile()
	compose, hasRuntime := detectCompose(p.look)

	if p.reach(address.host, address.port, p.probe) {
		return p.migrateAnsweredDatabase(composeFile, compose, hasRuntime, address, environ)
	}

	// Nothing answers. Everything below is about whether this folder can start
	// one at all, and each failure names the missing piece. The runtime is
	// checked before the password: telling somebody to set a password they
	// cannot use yet is a dead end.
	if composeFile == "" {
		return Result{}, ErrNoCompose
	}
	if !hasRuntime {
		return Result{}, ErrNoRuntime
	}
	if !p.hasSecret() {
		return Result{}, ErrNoPassword
	}

	if err := compose.run(p.ctx, p.dir, environ, p.out, "up", "-d", "db"); err != nil {
		return Result{}, err
	}

	if !p.waitFor(address) {
		return Result{}, fmt.Errorf("%w (%s)", ErrNoAnswer, address)
	}

	if err := compose.run(p.ctx, p.dir, environ, p.out, "run", "--rm", "migrate"); err != nil {
		return Result{Started: true}, err
	}

	return Result{Started: true, Migrated: true}, nil
}

// migrateAnsweredDatabase applies the migrations to a database that is already
// up, and leaves one that this folder's services do not own completely alone.
//
// The `migrate` service targets the Compose `db` over the Compose network, so it
// is only the right tool when the API is about to read that same database: a
// local address and the folder's own services. A database on another machine
// gets no migrations from here — its schema is its owner's business, and the
// window has no way to reach it with this folder's migration job.
func (p preparer) migrateAnsweredDatabase(composeFile string, compose composeRuntime, hasRuntime bool, address database, environ []string) (Result, error) {
	if composeFile == "" || !hasRuntime || !address.isLocal() {
		return Result{}, nil
	}

	if !p.hasSecret() {
		return Result{}, ErrNoPassword
	}

	if err := compose.run(p.ctx, p.dir, environ, p.out, "run", "--rm", "migrate"); err != nil {
		return Result{}, err
	}

	return Result{Migrated: true}, nil
}

// composeFile is the file this folder's services are declared in, or "" when
// there is none. Compose itself discovers all four names.
func (p preparer) composeFile() string {
	for _, name := range []string{ComposeFileName, "docker-compose.yaml", "compose.yaml", "compose.yml"} {
		info, err := os.Stat(filepath.Join(p.dir, name))
		if err == nil && !info.IsDir() {
			return name
		}
	}

	return ""
}

// hasSecret reports whether the folder carries the database password.
func (p preparer) hasSecret() bool {
	_, err := os.Stat(filepath.Join(p.dir, filepath.FromSlash(SecretPath)))

	return err == nil
}

// waitFor polls the address until something answers or the wait runs out: a
// database that has just been created needs a moment to accept connections.
func (p preparer) waitFor(address database) bool {
	poll := p.poll
	if poll <= 0 {
		poll = 250 * time.Millisecond
	}

	deadline := time.Now().Add(p.wait)

	for {
		if p.reach(address.host, address.port, p.probe) {
			return true
		}

		if time.Now().After(deadline) {
			return false
		}

		select {
		case <-p.ctx.Done():
			return false
		case <-time.After(poll):
		}
	}
}

// database is the address the API will use, from the folder's configuration.
type database struct {
	host string
	port string
}

func (d database) String() string {
	return net.JoinHostPort(d.host, d.port)
}

// isLocal reports whether this address is the machine itself, which is when the
// folder's `migrate` service — pointed at the Compose `db` — is the right tool.
func (d database) isLocal() bool {
	switch strings.ToLower(d.host) {
	case "127.0.0.1", "localhost", "::1", "[::1]":
		return true
	default:
		return false
	}
}

// databaseAddress reads the address the API will dial, with the defaults the
// compose file uses when the file says nothing.
func databaseAddress(env *EnvFile) database {
	address := database{host: defaultDBHost, port: defaultDBPort}

	if env == nil {
		return address
	}

	if host := strings.TrimSpace(env.Value("DB_HOST", "")); host != "" {
		address.host = host
	}
	if port := strings.TrimSpace(env.Value("DB_PORT", "")); port != "" {
		address.port = port
	}

	return address
}

// reachable reports whether something accepts a connection at host:port.
func reachable(host, port string, timeout time.Duration) bool {
	connection, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), timeout)
	if err != nil {
		return false
	}

	_ = connection.Close()

	return true
}

// composeRuntime is how this machine starts a folder's containers. The words in
// prefix follow the command, so `docker compose` and `podman-compose` are the
// same type.
type composeRuntime struct {
	command string
	prefix  []string
}

func (r composeRuntime) String() string {
	return strings.TrimSpace(r.command + " " + strings.Join(r.prefix, " "))
}

// detectCompose finds a container runtime, trying the spellings people have:
// Docker's compose plugin, the standalone podman-compose, the old
// docker-compose binary, then podman's own compose subcommand.
func detectCompose(look func(string) (string, error)) (composeRuntime, bool) {
	candidates := []composeRuntime{
		{command: "docker", prefix: []string{"compose"}},
		{command: "podman-compose"},
		{command: "docker-compose"},
		{command: "podman", prefix: []string{"compose"}},
	}

	for _, candidate := range candidates {
		if _, err := look(candidate.command); err == nil {
			return candidate, true
		}
	}

	return composeRuntime{}, false
}

// run runs one compose command in the folder, with its output going to the
// panel as it arrives. ctx bounds it: the process is killed when it is over.
func (r composeRuntime) run(ctx context.Context, dir string, environ []string, out io.Writer, args ...string) error {
	words := append(append([]string{}, r.prefix...), args...)

	command := exec.CommandContext(ctx, r.command, words...)
	command.Dir = dir
	command.Env = environ
	command.Stdout = out
	command.Stderr = out

	if err := command.Run(); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return fmt.Errorf("%s %s: %w", r, strings.Join(args, " "), ctxErr)
		}

		return fmt.Errorf("%s %s: %w", r, strings.Join(args, " "), err)
	}

	return nil
}
