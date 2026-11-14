package serverctl

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ServerBinaryName is the one binary a bundle carries: the client and the
// server are the same program. A bundle from an older release has the
// standalone server instead — see ServerBinary.
const ServerBinaryName = "eznit"

// ServerCommand is the argument that makes the one binary run the API.
const ServerCommand = "serve"

// legacyServerBinaryName is the standalone server a bundle used before the two
// were merged. It is still run when a folder has no eznit, so an archive
// somebody already extracted keeps starting.
const legacyServerBinaryName = "eznit-server"

// defaultPort is what the API listens on when API_PORT is unset.
const defaultPort = "4000"

// maxOutputLines bounds the log a supervisor remembers. A server that has been
// up for a week must not turn the window into a memory leak.
const maxOutputLines = 500

// State is what the window shows about the process.
type State struct {
	Running bool
	PID     int
	Exit    error
}

// Supervisor runs one server process from a bundle folder.
type Supervisor struct {
	mu    sync.Mutex
	dir   string
	cmd   *exec.Cmd
	done  chan struct{}
	exit  error
	lines []string
	rest  string // the tail of a line the process has not finished writing

	// written is how many lines the child has produced, ever. Output is
	// capped, so a window keeping its own copy needs the total to tell a
	// trimmed buffer from an unchanged one.
	written int

	// notifyMu guards the callback on its own: changed() runs while mu is
	// held (a start, a write), and the callback reads Output() through mu.
	notifyMu sync.Mutex
	notify   func()
}

// NewSupervisor returns a supervisor for the server in dir. Nothing is started
// until Start.
func NewSupervisor(dir string) *Supervisor {
	return &Supervisor{dir: dir}
}

// Dir is the bundle folder the supervisor works in.
func (s *Supervisor) Dir() string { return s.dir }

// SetNotify registers the callback that runs after the state or the output
// changes. A window passes a function that schedules its redraw; the callback
// runs on the goroutines that read the child, never on the UI thread.
func (s *Supervisor) SetNotify(fn func()) {
	s.notifyMu.Lock()
	s.notify = fn
	s.notifyMu.Unlock()
}

// State is a snapshot, safe to read while the process runs.
func (s *Supervisor) State() State {
	s.mu.Lock()
	defer s.mu.Unlock()

	state := State{Running: s.cmd != nil, Exit: s.exit}
	if s.cmd != nil && s.cmd.Process != nil {
		state.PID = s.cmd.Process.Pid
	}

	return state
}

// Output is a copy of the log, oldest line first.
func (s *Supervisor) Output() []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	lines := make([]string, len(s.lines))
	copy(lines, s.lines)

	return lines
}

// Written is how many lines the child has written in total. Output keeps only
// the last maxOutputLines, so a caller that keeps its own copy uses this to
// work out how many of the lines it shows have been dropped underneath it.
func (s *Supervisor) Written() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.written
}

// ClearOutput forgets the log so far.
func (s *Supervisor) ClearOutput() {
	s.mu.Lock()
	s.lines = nil
	s.rest = ""
	s.mu.Unlock()

	s.changed()
}

// Start launches the server. It returns once the process exists; whether it
// stays up is what the output and State report.
//
// The start is preflighted: the binary has to be in the folder, the .env is
// created from the example when it is missing, and the API port must be free.
// Those are the three failures a person can fix without reading a stack trace.
func (s *Supervisor) Start() error {
	if err := s.start(); err != nil {
		return err
	}

	// The notify runs outside the lock. A window's redraw reads State and
	// Output, so calling it while this method holds the mutex is a deadlock
	// waiting for a driver that runs the callback inline.
	s.changed()

	return nil
}

// start does the work with the supervisor's lock held.
func (s *Supervisor) start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cmd != nil {
		return errors.New("the server is already running")
	}

	binary, args, err := ServerBinary(s.dir)
	if err != nil {
		return err
	}

	env, err := LoadOrCreate(s.dir)
	if err != nil {
		return err
	}

	if err := portFree(env.Value("API_PORT", defaultPort)); err != nil {
		return err
	}

	cmd := exec.Command(binary, args...)
	cmd.Dir = s.dir
	// The .env wins over the inherited environment: it is the file the person
	// just edited, and godotenv in the child would not override what is
	// already set.
	cmd.Env = append(os.Environ(), env.ChildEnv()...)
	cmd.Stdout = s
	cmd.Stderr = s

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("cannot start %s: %w", binary, err)
	}

	s.cmd = cmd
	s.done = make(chan struct{})
	s.exit = nil

	done := s.done

	go func() {
		exit := cmd.Wait()

		s.mu.Lock()
		s.exit = exit
		s.cmd = nil
		s.mu.Unlock()

		close(done)
		s.changed()
	}()

	return nil
}

// Stop asks the server to shut down and waits for it. The interrupt is what
// the API drains on; Windows cannot deliver one to another process, so there
// the process is killed.
func (s *Supervisor) Stop(timeout time.Duration) error {
	s.mu.Lock()
	cmd, done := s.cmd, s.done
	s.mu.Unlock()

	if cmd == nil || done == nil {
		return nil
	}

	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		_ = cmd.Process.Kill()
	}

	select {
	case <-done:
		return nil
	case <-time.After(timeout):
		_ = cmd.Process.Kill()
		<-done
		return fmt.Errorf("the server did not stop within %s and was killed", timeout)
	}
}

// Write takes the child's output, one line at a time, so the window can show
// the reason a start failed instead of a button that did nothing.
func (s *Supervisor) Write(data []byte) (int, error) {
	s.mu.Lock()

	text := s.rest + string(data)

	if last := strings.LastIndexByte(text, '\n'); last >= 0 {
		s.rest = text[last+1:]
		text = text[:last]
	} else {
		s.rest = text
		text = ""
	}

	if text != "" {
		for _, line := range strings.Split(text, "\n") {
			s.lines = append(s.lines, strings.TrimSuffix(line, "\r"))
			s.written++
		}
	}

	if extra := len(s.lines) - maxOutputLines; extra > 0 {
		s.lines = append([]string(nil), s.lines[extra:]...)
	}

	s.mu.Unlock()

	s.changed()

	return len(data), nil
}

// changed tells the window that something moved.
func (s *Supervisor) changed() {
	s.notifyMu.Lock()
	notify := s.notify
	s.notifyMu.Unlock()

	if notify != nil {
		notify()
	}
}

// LocalURL is where the server answers on this machine, for the health check
// and the "open in a browser" button.
func (s *Supervisor) LocalURL() string {
	return LocalAddress(s.dir)
}

// LocalAddress is the API a server started in dir listens on: API_PORT from its
// configuration file, or the default when the file has none.
func LocalAddress(dir string) string {
	port := defaultPort

	if env, err := LoadEnv(filepath.Join(dir, EnvFileName)); err == nil {
		if value := env.Value("API_PORT", defaultPort); value != "" {
			port = value
		}
	}

	return "http://127.0.0.1:" + port
}

// ServerBinary is the executable in dir that can run the API, with the
// arguments it needs.
//
// `eznit serve` is the shipped form. A folder from before the two binaries were
// merged carries `eznit` too — as the client — so the name alone cannot say
// whether the API is in there: the binary is asked, with the `--help` that exits
// before the configuration, the database and a listener. A binary that cannot
// serve is skipped, and the standalone `eznit-server` beside it is used instead,
// which is what keeps a window updated in place working.
func ServerBinary(dir string) (path string, args []string, err error) {
	merged := filepath.Join(dir, BinaryName())
	legacy := filepath.Join(dir, legacyBinaryName())

	mergedThere := isBinaryFile(merged)
	legacyThere := isBinaryFile(legacy)

	if mergedThere && canServe(merged) {
		return merged, []string{ServerCommand}, nil
	}

	if legacyThere {
		return legacy, nil, nil
	}

	if mergedThere {
		return "", nil, fmt.Errorf("%s in %s cannot serve — it is older than the merged binary; download the release again", BinaryName(), dir)
	}

	return "", nil, fmt.Errorf("no %s in %s — this folder is not a server bundle", BinaryName(), dir)
}

// HasServerBinary reports whether the folder carries anything that could run
// the API, by name. It reads the folder and runs nothing: the panel asks this
// before a person presses anything, so a wrong folder is not executed.
func HasServerBinary(dir string) bool {
	return isBinaryFile(filepath.Join(dir, BinaryName())) ||
		isBinaryFile(filepath.Join(dir, legacyBinaryName()))
}

// isBinaryFile reports whether at is a file the folder could run.
func isBinaryFile(at string) bool {
	info, err := os.Stat(at)

	return err == nil && !info.IsDir()
}

// canServe asks the binary whether it has the `serve` command.
//
// `--help` exits before the configuration, the database and a listener, so this
// is a question any build can answer on any machine — and it is the only way to
// tell the merged binary from the client that came before it.
func canServe(binary string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	command := exec.CommandContext(ctx, binary, ServerCommand, "--help")
	command.Stdout = io.Discard
	command.Stderr = io.Discard

	return command.Run() == nil
}

// Health asks the running API's health check. It answers without a session and
// without touching the database, so it reports that the process is up and
// nothing more.
func Health(baseURL string, timeout time.Duration) error {
	client := &http.Client{Timeout: timeout}

	response, err := client.Get(strings.TrimSuffix(baseURL, "/") + "/v1/healthcheck")
	if err != nil {
		return err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("the health check answered %d", response.StatusCode)
	}

	return nil
}

// PortFree reports whether the API could bind the port the folder's
// configuration names.
//
// Start asks before it prepares the database: a server that cannot bind is not
// worth starting containers and applying migrations for, and the reason belongs
// in front of the person before the slow part.
func PortFree(dir string) error {
	port := defaultPort

	if env, err := LoadEnv(filepath.Join(dir, EnvFileName)); err == nil {
		if value := env.Value("API_PORT", ""); value != "" {
			port = value
		}
	}

	return portFree(port)
}

// portFree reports whether the API could bind the port it is configured for.
// The listener is closed at once: this is a check, not a reservation.
func portFree(port string) error {
	listener, err := net.Listen("tcp", ":"+port)
	if err != nil {
		return fmt.Errorf("port %s is already in use — stop what has it, or change API_PORT", port)
	}

	return listener.Close()
}
