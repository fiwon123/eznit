package serverctl

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

// fakeRuntime writes a script named name onto PATH that records its arguments in
// the returned log file and exits with the given status. Nothing here starts a
// container: the runtime is a shell script.
func fakeRuntime(t *testing.T, name string, exit int) (logFile string) {
	t.Helper()

	if runtime.GOOS == "windows" {
		t.Skip("the fake runtime is a shell script; Windows cannot run it")
	}

	dir := t.TempDir()
	logFile = filepath.Join(dir, "commands.log")

	script := "#!/bin/sh\necho \"$@\" >> " + logFile + "\necho \"fake: $@\"\nexit " + strconv.Itoa(exit) + "\n"
	if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	return logFile
}

// commands is what the fake runtime was asked to do, one line per call.
func commands(t *testing.T, logFile string) []string {
	t.Helper()

	data, err := os.ReadFile(logFile)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}

	return strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
}

// bundleFor writes the files the database step reads: a configuration naming a
// database address, and optionally the compose file and the password secret.
func bundleFor(t *testing.T, compose, secret bool) string {
	t.Helper()

	dir := t.TempDir()

	env := "DB_HOST=\"127.0.0.1\"\nDB_PORT=\"55432\"\n"
	if err := os.WriteFile(filepath.Join(dir, EnvFileName), []byte(env), 0o600); err != nil {
		t.Fatal(err)
	}

	if compose {
		if err := os.WriteFile(filepath.Join(dir, ComposeFileName), []byte("services: {}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	if secret {
		if err := os.MkdirAll(filepath.Join(dir, "secrets"), 0o700); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, filepath.FromSlash(SecretPath))
		if err := os.WriteFile(path, []byte("password\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	return dir
}

// noRuntime is a look function for a machine with no container runtime.
func noRuntime(string) (string, error) {
	return "", exec.ErrNotFound
}

// newPreparer is a preparer whose clock is short and whose runtime is a fake on
// PATH. answers is called for every probe, counting from 1.
func newPreparer(t *testing.T, dir string, answers func(call int) bool) (*preparer, *int, *bytes.Buffer) {
	t.Helper()

	output := &bytes.Buffer{}
	calls := new(int)

	p := &preparer{
		ctx:   context.Background(),
		dir:   dir,
		out:   output,
		probe: 50 * time.Millisecond,
		wait:  100 * time.Millisecond,
		poll:  time.Millisecond,
		reach: func(string, string, time.Duration) bool {
			*calls++

			return answers(*calls)
		},
		look: exec.LookPath,
	}

	return p, calls, output
}

// A database that answers still belongs to a bundle that may have been
// upgraded: the migrations run, and nothing is started.
func TestPrepareMigratesADatabaseThatAnswers(t *testing.T) {
	dir := bundleFor(t, true, true)
	logFile := fakeRuntime(t, "docker", 0)

	p, calls, _ := newPreparer(t, dir, func(int) bool { return true })

	result, err := p.run()
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}

	if result.Started {
		t.Error("Prepare() started a database that was already answering")
	}
	if !result.Migrated {
		t.Error("Prepare() did not run the migrations")
	}
	if got := commands(t, logFile); len(got) != 1 || got[0] != "compose run --rm migrate" {
		t.Errorf("the runtime was asked for %q, want one migration run", got)
	}
	if *calls != 1 {
		t.Errorf("the address was probed %d times, want 1", *calls)
	}
}

// A database that answers, in a folder with no compose file and no runtime, is
// left alone: it is somebody else's database and somebody else's migrations.
func TestPrepareLeavesAnExternalDatabaseAlone(t *testing.T) {
	for _, tt := range []struct {
		name    string
		compose bool
		runtime bool
	}{
		{"no compose file, no runtime", false, false},
		{"a compose file but no runtime", true, false},
		{"a runtime but no compose file", false, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := bundleFor(t, tt.compose, true)

			p, _, _ := newPreparer(t, dir, func(int) bool { return true })
			if tt.runtime {
				fakeRuntime(t, "docker", 0)
			} else {
				p.look = noRuntime
			}

			result, err := p.run()
			if err != nil {
				t.Fatalf("Prepare() error = %v", err)
			}
			if result.Started || result.Migrated {
				t.Errorf("Prepare() reported %+v, want nothing done to a database it did not start", result)
			}
		})
	}
}

// The whole point: nothing answers, and the folder has everything, so the
// database comes up and the migrations are applied.
func TestPrepareStartsTheDatabase(t *testing.T) {
	dir := bundleFor(t, true, true)
	logFile := fakeRuntime(t, "docker", 0)

	p, _, output := newPreparer(t, dir, func(call int) bool { return call > 1 })

	result, err := p.run()
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}

	if !result.Started || !result.Migrated {
		t.Errorf("Prepare() = %+v, want the database started and migrated", result)
	}

	want := []string{"compose up -d db", "compose run --rm migrate"}
	if got := commands(t, logFile); len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("the runtime was asked for %q, want %q", got, want)
	}

	if !strings.Contains(output.String(), "compose up -d db") {
		t.Errorf("the runtime's output did not reach the caller: %q", output.String())
	}
}

// Each missing piece is its own error, because each one has a different fix.
func TestPrepareNamesWhatIsMissing(t *testing.T) {
	tests := []struct {
		name    string
		compose bool
		secret  bool
		runtime bool
		want    error
	}{
		{"no compose file", false, true, true, ErrNoCompose},
		{"no password file", true, false, true, ErrNoPassword},
		{"no container runtime", true, true, false, ErrNoRuntime},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := bundleFor(t, tt.compose, tt.secret)

			p, _, _ := newPreparer(t, dir, func(int) bool { return false })
			if tt.runtime {
				fakeRuntime(t, "docker", 0)
			} else {
				p.look = noRuntime
			}

			_, err := p.run()

			if !errors.Is(err, tt.want) {
				t.Errorf("Prepare() error = %v, want %v", err, tt.want)
			}
		})
	}
}

// A runtime that fails stops the start: the caller keeps its output, which is
// the only place the reason is written down.
func TestPrepareReportsAFailedStart(t *testing.T) {
	dir := bundleFor(t, true, true)
	logFile := fakeRuntime(t, "docker", 1)

	p, _, _ := newPreparer(t, dir, func(int) bool { return false })

	_, err := p.run()

	if err == nil {
		t.Fatal("Prepare() = nil, want the runtime's failure")
	}
	if !strings.Contains(err.Error(), "up -d db") {
		t.Errorf("error = %q, want it to name the command that failed", err)
	}
	if got := commands(t, logFile); len(got) != 1 {
		t.Errorf("the runtime was asked for %q, want only the start", got)
	}
}

// A database that starts and never answers is a wait, then a named failure: the
// API must not be pointed at it.
func TestPrepareWaitsThenReportsADatabaseThatNeverAnswers(t *testing.T) {
	dir := bundleFor(t, true, true)
	fakeRuntime(t, "docker", 0)

	p, calls, _ := newPreparer(t, dir, func(int) bool { return false })

	start := time.Now()

	_, err := p.run()

	if !errors.Is(err, ErrNoAnswer) {
		t.Fatalf("Prepare() error = %v, want %v", err, ErrNoAnswer)
	}
	if *calls < 2 {
		t.Errorf("the address was probed %d times, want the wait to poll it", *calls)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("the wait took %s, want it bounded by the deadline", elapsed)
	}
}

// A migration that fails after the database came up is still a failed start,
// and the result says which half happened.
func TestPrepareReportsAFailedMigration(t *testing.T) {
	dir := bundleFor(t, true, true)

	// Exits 0 for the start and 1 for the migration: `docker compose run`.
	bin := t.TempDir()
	script := "#!/bin/sh\nif [ \"$2\" = run ]; then exit 1; fi\nexit 0\n"
	if err := os.WriteFile(filepath.Join(bin, "docker"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	p, _, _ := newPreparer(t, dir, func(call int) bool { return call > 1 })

	result, err := p.run()

	if err == nil {
		t.Fatal("Prepare() = nil, want the migration's failure")
	}
	if !result.Started {
		t.Errorf("Prepare() = %+v, want it to report the database it did start", result)
	}
	if result.Migrated {
		t.Error("Prepare() claimed a migration that failed")
	}
}

// The runtime is found under every spelling people have, in the order that
// keeps the machine's own preference.
func TestDetectCompose(t *testing.T) {
	found := func(names ...string) func(string) (string, error) {
		return func(name string) (string, error) {
			for _, candidate := range names {
				if candidate == name {
					return "/usr/bin/" + name, nil
				}
			}

			return "", exec.ErrNotFound
		}
	}

	tests := []struct {
		name  string
		found []string
		want  string
		ok    bool
	}{
		{"docker's own compose", []string{"docker", "podman"}, "docker compose", true},
		{"podman-compose before podman", []string{"podman", "podman-compose"}, "podman-compose", true},
		{"the old docker-compose", []string{"docker-compose"}, "docker-compose", true},
		{"podman's compose subcommand", []string{"podman"}, "podman compose", true},
		{"nothing installed", nil, "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runtime, ok := detectCompose(found(tt.found...))

			if ok != tt.ok {
				t.Fatalf("detectCompose() ok = %v, want %v", ok, tt.ok)
			}
			if ok && runtime.String() != tt.want {
				t.Errorf("detectCompose() = %q, want %q", runtime, tt.want)
			}
		})
	}
}

// The address the API will dial comes from the configuration, with the compose
// file's defaults when it says nothing.
func TestDatabaseAddress(t *testing.T) {
	tests := []struct {
		name string
		env  string
		want string
	}{
		{"the configured address", "DB_HOST=\"db.example\"\nDB_PORT=\"6543\"\n", "db.example:6543"},
		{"defaults when the file is empty", "", "127.0.0.1:5432"},
		{"a host on its own", "DB_HOST=\"10.0.0.5\"\n", "10.0.0.5:5432"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, EnvFileName), []byte(tt.env), 0o600); err != nil {
				t.Fatal(err)
			}

			file, err := LoadEnv(filepath.Join(dir, EnvFileName))
			if err != nil {
				t.Fatal(err)
			}

			if got := databaseAddress(file).String(); got != tt.want {
				t.Errorf("databaseAddress() = %q, want %q", got, tt.want)
			}
		})
	}
}

// A real dial against a listener is reachable; a closed port is not.
func TestReachable(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := strconv.Itoa(listener.Addr().(*net.TCPAddr).Port)

	if !reachable("127.0.0.1", port, time.Second) {
		t.Error("a listening port was reported unreachable")
	}

	listener.Close()

	if reachable("127.0.0.1", port, 200*time.Millisecond) {
		t.Error("a closed port was reported reachable")
	}
}

// A folder that is not a bundle cannot even be read, and the error says so.
func TestPrepareNeedsAConfiguration(t *testing.T) {
	_, err := Prepare(context.Background(), t.TempDir(), io.Discard)

	if err == nil || !strings.Contains(err.Error(), "not a server bundle") {
		t.Errorf("Prepare() error = %v, want it to say the folder is not a bundle", err)
	}
}

// The runtime runs in the folder: that is what makes `docker compose` find the
// compose file and the `.env` beside it.
func TestPrepareRunsTheRuntimeInTheFolder(t *testing.T) {
	dir := bundleFor(t, true, true)

	bin := t.TempDir()
	report := filepath.Join(bin, "cwd.txt")
	script := "#!/bin/sh\npwd > " + report + "\nexit 0\n"
	if err := os.WriteFile(filepath.Join(bin, "docker"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	p, _, _ := newPreparer(t, dir, func(call int) bool { return call > 1 })

	if _, err := p.run(); err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}

	got, err := os.ReadFile(report)
	if err != nil {
		t.Fatalf("the runtime never ran: %v", err)
	}
	if wanted := strings.TrimSpace(string(got)); wanted != dir {
		t.Errorf("the runtime ran in %q, want the bundle folder %q", wanted, dir)
	}
}

// The folder's `.env` wins over the environment the window was launched from:
// Compose substitutes from the environment, and the probe must look at the
// database the containers will actually use.
func TestPrepareLetsTheFolderWinOverTheEnvironment(t *testing.T) {
	dir := bundleFor(t, true, true)

	// The window's own environment names a different port than the file.
	t.Setenv("DB_PORT", "9999")

	bin := t.TempDir()
	report := filepath.Join(bin, "port.txt")
	script := "#!/bin/sh\nif [ -z \"$REPORTED\" ]; then echo \"$DB_PORT\" > " + report + "; REPORTED=1; fi\nexit 0\n"
	if err := os.WriteFile(filepath.Join(bin, "docker"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	p, _, _ := newPreparer(t, dir, func(call int) bool { return call > 1 })

	if _, err := p.run(); err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}

	got, err := os.ReadFile(report)
	if err != nil {
		t.Fatalf("the runtime never ran: %v", err)
	}
	if value := strings.TrimSpace(string(got)); value != "55432" {
		t.Errorf("the runtime saw DB_PORT=%q, want the folder's 55432", value)
	}
}

// A database on another machine is not the one this folder's migrate service
// targets, so the service must not run against it.
func TestPrepareLeavesARemoteDatabaseUnmigrated(t *testing.T) {
	dir := bundleFor(t, true, true)
	if err := os.WriteFile(filepath.Join(dir, EnvFileName), []byte("DB_HOST=\"10.1.2.3\"\nDB_PORT=\"5432\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	logFile := fakeRuntime(t, "docker", 0)

	p, _, _ := newPreparer(t, dir, func(int) bool { return true })

	result, err := p.run()
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}

	if result.Migrated || result.Started {
		t.Errorf("Prepare() = %+v, want a remote database left alone", result)
	}
	if got := commands(t, logFile); len(got) != 0 {
		t.Errorf("the runtime was asked for %q, want nothing", got)
	}
}

// A database that answers still needs the secret if the folder's migrate
// service is going to run: Compose cannot mount a file that is not there.
func TestPrepareNeedsTheSecretToMigrate(t *testing.T) {
	dir := bundleFor(t, true, false)
	fakeRuntime(t, "docker", 0)

	p, _, _ := newPreparer(t, dir, func(int) bool { return true })

	_, err := p.run()

	if !errors.Is(err, ErrNoPassword) {
		t.Errorf("Prepare() error = %v, want %v", err, ErrNoPassword)
	}
}

// The runtime is named before the password: telling somebody to set a password
// they have no way to use yet is a dead end.
func TestPrepareNamesTheRuntimeBeforeThePassword(t *testing.T) {
	dir := bundleFor(t, true, false)

	p, _, _ := newPreparer(t, dir, func(int) bool { return false })
	p.look = noRuntime

	_, err := p.run()

	if !errors.Is(err, ErrNoRuntime) {
		t.Errorf("Prepare() error = %v, want %v", err, ErrNoRuntime)
	}
}

// Compose discovers four file names, and a folder using another one is
// startable all the same.
func TestPrepareAcceptsEveryComposeFileName(t *testing.T) {
	for _, name := range []string{"docker-compose.yml", "docker-compose.yaml", "compose.yaml", "compose.yml"} {
		t.Run(name, func(t *testing.T) {
			dir := bundleFor(t, false, true)
			if err := os.WriteFile(filepath.Join(dir, name), []byte("services: {}\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			fakeRuntime(t, "docker", 0)

			p, _, _ := newPreparer(t, dir, func(call int) bool { return call > 1 })

			result, err := p.run()
			if err != nil {
				t.Fatalf("Prepare() error = %v", err)
			}
			if !result.Started {
				t.Errorf("Prepare() = %+v, want the database started from %s", result, name)
			}
		})
	}
}

// Cancelling the step ends it, kills the child, and returns the cancellation
// the panel reports: a hung daemon must not hold the window.
func TestPrepareStopsWhenCancelled(t *testing.T) {
	dir := bundleFor(t, true, true)

	// A runtime that ignores everything and never exits.
	bin := t.TempDir()
	script := "#!/bin/sh\nexec sleep 30\n"
	if err := os.WriteFile(filepath.Join(bin, "docker"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	ctx, cancel := context.WithCancel(context.Background())
	p, _, _ := newPreparer(t, dir, func(int) bool { return false })
	p.ctx = ctx
	p.wait = time.Minute

	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	start := time.Now()

	_, err := p.run()

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Prepare() error = %v, want the cancellation", err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("the cancel took %s, want it to end the child at once", elapsed)
	}
}

// PortFree reports the port the folder's configuration names, and refuses one
// something else holds.
func TestPortFree(t *testing.T) {
	dir := t.TempDir()
	writeEnv(t, dir, freePort(t))

	if err := PortFree(dir); err != nil {
		t.Fatalf("PortFree() = %v, want a free port reported free", err)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	port := strconv.Itoa(listener.Addr().(*net.TCPAddr).Port)
	writeEnv(t, dir, port)

	if err := PortFree(dir); err == nil {
		t.Error("PortFree() = nil, want the busy port reported")
	}
}
