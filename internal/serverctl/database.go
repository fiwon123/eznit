package serverctl

import (
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
	// ErrNoCompose is a folder with no docker-compose.yml: the file the
	// migrations and the database are started from.
	ErrNoCompose = errors.New("this folder has no docker-compose.yml")

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

// ComposeFileName is the file the database and the migrations are started from.
// A folder without it cannot start either.
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
// out receives the container runtime's output, line by line, as it arrives.
func Prepare(dir string, out io.Writer) (Result, error) {
	return preparer{
		dir:   dir,
		out:   out,
		probe: 750 * time.Millisecond,
		wait:  30 * time.Second,
		poll:  250 * time.Millisecond,
		reach: reachable,
		look:  exec.LookPath,
	}.run()
}

// preparer is Prepare with its clock and its seams visible, so a test can drive
// every branch without a database or a container runtime.
type preparer struct {
	dir   string
	out   io.Writer
	probe time.Duration
	wait  time.Duration
	poll  time.Duration
	reach func(host, port string, timeout time.Duration) bool
	look  func(string) (string, error)
}

func (p preparer) run() (Result, error) {
	// The .env is where the address comes from, and a first start creates it
	// from the example — the same call the supervisor makes, so both agree.
	env, err := LoadOrCreate(p.dir)
	if err != nil {
		return Result{}, err
	}

	address := databaseAddress(env)

	if p.reach(address.host, address.port, p.probe) {
		// Answering is not the same as current: an upgraded bundle adds
		// migrations, and `migrate up` is idempotent.
		compose, hasRuntime := detectCompose(p.look)
		if !hasRuntime || !p.hasComposeFile() {
			return Result{}, nil
		}

		if err := compose.run(p.dir, p.out, "run", "--rm", "migrate"); err != nil {
			return Result{}, err
		}

		return Result{Migrated: true}, nil
	}

	// Nothing answers. Everything below is about whether this folder can start
	// one at all, and each failure names the missing piece.
	if !p.hasComposeFile() {
		return Result{}, ErrNoCompose
	}

	if _, err := os.Stat(filepath.Join(p.dir, filepath.FromSlash(SecretPath))); err != nil {
		return Result{}, ErrNoPassword
	}

	compose, ok := detectCompose(p.look)
	if !ok {
		return Result{}, ErrNoRuntime
	}

	if err := compose.run(p.dir, p.out, "up", "-d", "db"); err != nil {
		return Result{}, err
	}

	if !p.waitFor(address) {
		return Result{}, fmt.Errorf("%w (%s)", ErrNoAnswer, address)
	}

	if err := compose.run(p.dir, p.out, "run", "--rm", "migrate"); err != nil {
		return Result{Started: true}, err
	}

	return Result{Started: true, Migrated: true}, nil
}

// hasComposeFile reports whether this folder carries the services the database
// and the migrations are started from.
func (p preparer) hasComposeFile() bool {
	info, err := os.Stat(filepath.Join(p.dir, ComposeFileName))

	return err == nil && !info.IsDir()
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

		time.Sleep(poll)
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
// panel as it arrives.
func (r composeRuntime) run(dir string, out io.Writer, args ...string) error {
	words := append(append([]string{}, r.prefix...), args...)

	command := exec.Command(r.command, words...)
	command.Dir = dir
	command.Stdout = out
	command.Stderr = out

	if err := command.Run(); err != nil {
		return fmt.Errorf("%s %s: %w", r, strings.Join(args, " "), err)
	}

	return nil
}
