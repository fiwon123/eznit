package serverctl

import (
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

// writeFakeServer puts a shell script where the supervisor looks for the
// server, so a test drives the real process machinery without a database.
//
// It answers `serve --help` the way the merged binary does — the supervisor asks
// that to tell a server from a client that predates it — and runs script for
// the real start.
func writeFakeServer(t *testing.T, dir, script string) {
	t.Helper()

	if runtime.GOOS == "windows" {
		t.Skip("the fake server is a shell script; Windows builds cannot run it")
	}

	body := "#!/bin/sh\n" +
		"if [ \"$1\" = " + ServerCommand + " ] && [ \"$2\" = --help ]; then exit 0; fi\n" +
		script

	path := filepath.Join(dir, ServerBinaryName)
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
}

// freePort asks the kernel for a port and gives it back.
func freePort(t *testing.T) string {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := strconv.Itoa(listener.Addr().(*net.TCPAddr).Port)
	listener.Close()

	return port
}

// writeEnv puts a .env in dir with the given API_PORT.
func writeEnv(t *testing.T, dir, port string) {
	t.Helper()

	content := "# test\nAPI_PORT=\"" + port + "\"\n"
	if err := os.WriteFile(filepath.Join(dir, EnvFileName), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// waitFor waits for a line in the output, so a test never races the child.
func waitFor(t *testing.T, supervisor *Supervisor, line string) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		for _, got := range supervisor.Output() {
			if strings.Contains(got, line) {
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}

	t.Fatalf("the output never contained %q; it holds %q", line, supervisor.Output())
}

func TestStartAndStopRunTheServer(t *testing.T) {
	dir := t.TempDir()
	writeEnv(t, dir, freePort(t))
	writeFakeServer(t, dir, "echo ready\ntrap 'echo stopping; exit 0' INT TERM\nwhile true; do sleep 0.05; done\n")

	supervisor := NewSupervisor(dir)

	if err := supervisor.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	waitFor(t, supervisor, "ready")

	if state := supervisor.State(); !state.Running || state.PID == 0 {
		t.Fatalf("State = %+v, want a running process with a pid", state)
	}

	if err := supervisor.Stop(5 * time.Second); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	waitFor(t, supervisor, "stopping")

	if state := supervisor.State(); state.Running {
		t.Error("State still reports the process as running after Stop")
	}
}

func TestStartTwiceIsRefused(t *testing.T) {
	dir := t.TempDir()
	writeEnv(t, dir, freePort(t))
	writeFakeServer(t, dir, "echo ready\nwhile true; do sleep 0.05; done\n")

	supervisor := NewSupervisor(dir)

	if err := supervisor.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = supervisor.Stop(5 * time.Second) })

	if err := supervisor.Start(); err == nil {
		t.Fatal("the second Start was accepted while the first ran")
	}
}

// The port check is what turns "it started and died" into an answer a person
// can act on.
func TestStartReportsABusyPort(t *testing.T) {
	dir := t.TempDir()

	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	port := strconv.Itoa(listener.Addr().(*net.TCPAddr).Port)
	writeEnv(t, dir, port)
	writeFakeServer(t, dir, "echo ready\nwhile true; do sleep 0.05; done\n")

	supervisor := NewSupervisor(dir)

	err = supervisor.Start()
	if err == nil {
		t.Fatal("Start accepted a port that is already bound")
	}
	if !strings.Contains(err.Error(), "already in use") {
		t.Errorf("error = %q, want it to name the busy port", err)
	}

	if state := supervisor.State(); state.Running {
		t.Error("a refused start left a process running")
	}
}

func TestStartWithoutABundleExplainsItself(t *testing.T) {
	dir := t.TempDir()

	supervisor := NewSupervisor(dir)

	err := supervisor.Start()
	if err == nil {
		t.Fatal("Start accepted a folder with no server binary")
	}
	if !strings.Contains(err.Error(), "not a server bundle") {
		t.Errorf("error = %q, want it to say the folder is not a bundle", err)
	}
}

// A stop that is ignored is a kill, and the caller is told which happened.
func TestStopReportsAProcessThatWouldNotDrain(t *testing.T) {
	dir := t.TempDir()
	writeEnv(t, dir, freePort(t))
	writeFakeServer(t, dir, "echo ready\ntrap '' INT TERM\nwhile true; do sleep 0.05; done\n")

	supervisor := NewSupervisor(dir)

	if err := supervisor.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	waitFor(t, supervisor, "ready")

	err := supervisor.Stop(200 * time.Millisecond)
	if err == nil {
		t.Fatal("Stop reported success for a process that ignored the interrupt")
	}
	if !strings.Contains(err.Error(), "killed") {
		t.Errorf("error = %q, want it to say the process was killed", err)
	}

	if state := supervisor.State(); state.Running {
		t.Error("the process is still running after Stop")
	}
}

func TestOutputKeepsTheLastLinesAndCanBeCleared(t *testing.T) {
	supervisor := NewSupervisor(t.TempDir())

	for i := 0; i < maxOutputLines+50; i++ {
		if _, err := supervisor.Write([]byte("line\n")); err != nil {
			t.Fatal(err)
		}
	}

	if got := len(supervisor.Output()); got != maxOutputLines {
		t.Errorf("Output holds %d lines, want %d", got, maxOutputLines)
	}

	supervisor.ClearOutput()

	if got := supervisor.Output(); len(got) != 0 {
		t.Errorf("Output after ClearOutput = %q, want empty", got)
	}
}

// A chunk that ends mid-line must not be shown as a line of its own: the rest
// arrives with the next write.
func TestOutputJoinsAPartialLine(t *testing.T) {
	supervisor := NewSupervisor(t.TempDir())

	_, _ = supervisor.Write([]byte(`{"level":"INFO","msg":"Ser`))
	_, _ = supervisor.Write([]byte("ver Running...\"}\n"))

	output := supervisor.Output()
	if len(output) != 1 {
		t.Fatalf("Output = %q, want one joined line", output)
	}
	if output[0] != `{"level":"INFO","msg":"Server Running..."}` {
		t.Errorf("the line was split: %q", output[0])
	}
}

func TestLocalURLUsesTheConfiguredPort(t *testing.T) {
	dir := t.TempDir()
	writeEnv(t, dir, "4321")

	if got := NewSupervisor(dir).LocalURL(); got != "http://127.0.0.1:4321" {
		t.Errorf("LocalURL = %q, want %q", got, "http://127.0.0.1:4321")
	}

	if got := NewSupervisor(t.TempDir()).LocalURL(); got != "http://127.0.0.1:4000" {
		t.Errorf("LocalURL without a .env = %q, want the default port", got)
	}
}

func TestHealthReportsAnAnsweringAndASilentServer(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := newHealthServer()
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close() })

	if err := Health("http://"+listener.Addr().String(), time.Second); err != nil {
		t.Errorf("Health against a live server = %v, want nil", err)
	}

	port := freePort(t)
	if err := Health("http://127.0.0.1:"+port, 200*time.Millisecond); err == nil {
		t.Error("Health against a closed port = nil, want an error")
	}
}

// newHealthServer is the answer the API gives a supervisor's health check.
func newHealthServer() *http.Server {
	return &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/healthcheck" {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"available"}`))
	})}
}

// Written counts the lines ever produced, not the ones still buffered: a
// window that keeps its own copy needs the difference to know how many rows to
// drop when the buffer is trimmed.
func TestWrittenCountsPastTheBuffer(t *testing.T) {
	supervisor := NewSupervisor(t.TempDir())

	total := maxOutputLines + 5
	for range total {
		if _, err := supervisor.Write([]byte("one line\n")); err != nil {
			t.Fatal(err)
		}
	}

	if got := supervisor.Written(); got != total {
		t.Errorf("Written() = %d, want %d", got, total)
	}
	if got := len(supervisor.Output()); got != maxOutputLines {
		t.Errorf("Output() holds %d lines, want the cap of %d", got, maxOutputLines)
	}

	supervisor.ClearOutput()

	if got := supervisor.Written(); got != total {
		t.Errorf("Written() after a clear = %d, want the total %d", got, total)
	}
	if got := len(supervisor.Output()); got != 0 {
		t.Errorf("Output() after a clear holds %d lines, want none", got)
	}
}

// The notify callback must run without the supervisor's lock: a window's
// redraw reads State and Output, and a driver that runs the callback inline
// (the test driver does) would otherwise deadlock on the lock Start holds.
func TestStartNotifiesWithoutHoldingTheLock(t *testing.T) {
	dir := t.TempDir()
	writeFakeServer(t, dir, "exec sleep 30\n")
	writeEnv(t, dir, freePort(t))

	supervisor := NewSupervisor(dir)

	notified := make(chan struct{}, 4)
	supervisor.SetNotify(func() {
		_ = supervisor.State()
		_ = supervisor.Output()

		select {
		case notified <- struct{}{}:
		default:
		}
	})

	started := make(chan error, 1)
	go func() { started <- supervisor.Start() }()

	select {
	case err := <-started:
		if err != nil {
			t.Fatalf("Start() = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Start() did not return: the notify ran while the lock was held")
	}

	select {
	case <-notified:
	case <-time.After(5 * time.Second):
		t.Fatal("the notify never ran after a start")
	}

	_ = supervisor.Stop(2 * time.Second)
}

// The supervisor runs the one binary with `serve`: without the argument the
// process would print the CLI help and exit, which is a start that looks like a
// crash.
func TestSupervisorRunsTheServeCommand(t *testing.T) {
	dir := t.TempDir()
	writeEnv(t, dir, freePort(t))

	report := filepath.Join(dir, "args.txt")
	writeFakeServer(t, dir, "echo \"$@\" > "+report+"\necho ready\nwhile true; do sleep 0.05; done\n")

	supervisor := NewSupervisor(dir)

	if err := supervisor.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = supervisor.Stop(5 * time.Second) })

	waitFor(t, supervisor, "ready")

	data, err := os.ReadFile(report)
	if err != nil {
		t.Fatalf("the server never ran: %v", err)
	}
	if got := strings.TrimSpace(string(data)); got != ServerCommand {
		t.Errorf("the server was started with %q, want %q", got, ServerCommand)
	}
}

// A folder from an older release runs its standalone server with no argument:
// the merged name is there (as the old client) and cannot serve, so the
// fallback is what keeps that folder starting.
func TestSupervisorStartsTheStandaloneServer(t *testing.T) {
	dir := t.TempDir()
	writeEnv(t, dir, freePort(t))

	legacy := filepath.Join(dir, legacyBinaryName())
	if err := os.WriteFile(legacy, []byte("#!/bin/sh\necho ready\nwhile true; do sleep 0.05; done\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	if runtime.GOOS == "windows" {
		t.Skip("the fake server is a shell script; Windows builds cannot run it")
	}

	supervisor := NewSupervisor(dir)

	if err := supervisor.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = supervisor.Stop(5 * time.Second) })

	waitFor(t, supervisor, "ready")
}
