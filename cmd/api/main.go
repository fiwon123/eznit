package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/fiwon123/eznit/internal/platform/api"
	"github.com/fiwon123/eznit/internal/platform/sql"
	"github.com/fiwon123/eznit/internal/platform/webui"
	"github.com/fiwon123/eznit/pkg/logger"
	"github.com/fiwon123/eznit/web"
	"github.com/joho/godotenv"
)

// Version is what `eznit-server --version` prints. It is set at build time:
//
//	go build -ldflags "-X main.Version=v1.2.3" ./cmd/api
//
// The Makefile passes it (INJECT_VERSION), so a released server names the tag it
// was built from, the way `eznit -v` and `eznit-gui --version` do. Without it
// the binary says "dev", which is honest: it was not built as a release.
//
// Declaring the variable is what makes the injection land. The linker accepts an
// -X for a symbol that does not exist and says nothing about it, so a Makefile
// that injects into a missing name looks like it works while doing nothing.
var Version = "dev"

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

// options are the flags `eznit-server` accepts.
type options struct {
	debug   bool
	version bool
}

// parseArgs reads the server's flags out of args.
//
// The arguments are handed in rather than read from os.Args so the flags can be
// tested without a database, a log folder or a listener — and so --version is
// answered before any of those start. The flag set reports errors instead of
// exiting, which is what keeps -h testable: flag.ExitOnError would end the
// process, and a test binary with it.
func parseArgs(args []string, out io.Writer) (options, error) {
	var opts options

	flags := flag.NewFlagSet("eznit-server", flag.ContinueOnError)
	flags.SetOutput(out)
	flags.BoolVar(&opts.debug, "debug", false, "show debug logs")
	flags.BoolVar(&opts.version, "version", false, "print the version and exit")

	if err := flags.Parse(args); err != nil {
		return options{}, err
	}

	return opts, nil
}

// run holds main's body so the defers it takes — the log flush, the database
// connection — run on every exit path, a failed start included.
func run() error {
	// Flags come first. `--version` has to answer on a machine with no .env, no
	// database and no log folder, and a mistyped flag should fail before
	// anything starts rather than halfway through a start-up.
	opts, err := parseArgs(os.Args[1:], os.Stderr)
	if errors.Is(err, flag.ErrHelp) {
		return nil // -h printed its own usage, and asking for help is not a failure
	}
	if err != nil {
		// The flag set already named the problem and printed the usage; a
		// second copy from log.Fatal would not help. Exit 2 is what the flag
		// package exits with for a usage error, and nothing is open yet, so no
		// deferred work is skipped.
		os.Exit(2)
	}

	if opts.version {
		fmt.Println(Version)
		return nil
	}

	// .env.local overwrite .env for development
	_ = godotenv.Load(".env.local", ".env")

	logsFolder, _ := os.LookupEnv("API_LOGS")
	if logsFolder == "" {
		logsFolder = "./logs/"
	}
	logger, err := logger.NewJson(logsFolder, opts.debug)
	if err != nil {
		return err
	}
	defer logger.Sync()

	db, err := sql.Open("postgres", sql.DSNFromEnv())
	if err != nil {
		logger.Error("Unable to connect to PostgreSQL!")
		return err
	}
	defer db.Close()

	logger.Info("Connected to PostgreSQL!")

	r := api.NewRouter(api.Config{
		DB:           db,
		Logger:       logger,
		UploadFolder: os.Getenv("API_UPLOADS"),
		RateLimit:    api.RateLimitFromEnv(),
	})

	logger.Info("Initialized routes!")

	// The web UI, when there is one. In development the Vite dev server
	// proxies /v1 to this API; in production this process serves the UI from
	// web/dist, or from the copy embedded at build time. One origin either
	// way, so the API needs no CORS headers.
	//
	// A deployment with a separate web tier (the `web` service in
	// docker-compose.yml) turns this off: then :4000 is the API only, and the
	// browser has exactly one frontend origin to talk to.
	var handler http.Handler = r
	if !webUIEnabled() {
		logger.Info("API_SERVE_WEB is off — serving the API only; the web UI is a separate service")
	} else if ui, source := pickWebUI(os.DirFS(webDistFolder()), web.FS(), r); ui != nil {
		logger.Info("Serving the web UI from " + source)
		handler = ui
	} else {
		logger.Info("No web UI found in " + webDistFolder() + " and none built in — serving the API only")
	}

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", apiPort()),
		Handler: handler,
	}

	// Bind before announcing the server. A busy port is then a start failure
	// the caller can report, not a process that says it is up and dies — the
	// desktop control panel tells those two apart.
	listener, err := net.Listen("tcp", srv.Addr)
	if err != nil {
		logger.Error("Unable to listen on " + srv.Addr)
		return err
	}

	// Ctrl-C, and the SIGTERM that `docker stop` or a service manager sends on
	// Unix, ask for a drain rather than a kill: serveUntil gives the requests
	// already running a bounded chance to finish.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	logger.Info("Server Running...")

	if err := serveUntil(ctx, srv, listener, shutdownTimeout); err != nil {
		logger.Error("The server stopped with an error: " + err.Error())
		return err
	}

	logger.Info("Server Stopped")

	return nil
}

// shutdownTimeout is how long in-flight requests get to finish after a stop
// signal.
const shutdownTimeout = 10 * time.Second

// serveUntil serves requests from listener until ctx is cancelled, then drains
// for up to drain and returns nil when everything finished in time.
//
// A stop is not a kill: Shutdown closes the listener and waits for the handlers
// that are already running, which is what keeps an upload from being cut in
// half by Ctrl-C. A drain that overruns closes the connections and returns the
// error, so the process can still exit.
func serveUntil(ctx context.Context, srv *http.Server, listener net.Listener, drain time.Duration) error {
	served := make(chan error, 1)
	go func() {
		served <- srv.Serve(listener)
	}()

	select {
	case err := <-served:
		// Serve returned on its own: the listener failed under us, which is a
		// real error, or the server was closed already, which is not.
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
	}

	drainCtx, cancel := context.WithTimeout(context.Background(), drain)
	defer cancel()

	if err := srv.Shutdown(drainCtx); err != nil {
		_ = srv.Close()
		return fmt.Errorf("the server did not drain within %s: %w", drain, err)
	}

	return nil
}

// apiPort is the port the API listens on.
//
// It defaults to 4000 so the CLI, the GUI and existing scripts keep working
// without configuration. API_PORT overrides it for a deployment that needs a
// different bind port.
func apiPort() int {
	value := strings.TrimSpace(os.Getenv("API_PORT"))
	if value == "" {
		return 4000
	}

	port, err := strconv.Atoi(value)
	if err != nil {
		return 4000
	}

	if port <= 0 || port > 65535 {
		return 4000
	}

	return port
}

// webDistFolder is where the built web UI lives. It is relative to the
// working directory of the API, and API_WEB_DIST moves it.
func webDistFolder() string {
	if dir := os.Getenv("API_WEB_DIST"); dir != "" {
		return dir
	}
	return "web/dist"
}

// webUIEnabled reports whether this process should serve the web UI.
//
// The default is on: the released single binary stays self-contained, and a
// developer running the API alone still gets the UI from web/dist. A Compose
// deployment with a separate `web` service sets API_SERVE_WEB=false, so :4000
// stays the API only — one port, one job.
//
// An unparsable value keeps the UI on rather than silently dropping it: a typo
// in a deployment variable must not turn a working single binary into an
// API-only one.
func webUIEnabled() bool {
	value := strings.TrimSpace(os.Getenv("API_SERVE_WEB"))
	if value == "" {
		return true
	}

	enabled, err := strconv.ParseBool(value)
	if err != nil {
		return true
	}

	return enabled
}

// pickWebUI chooses the web UI to serve in front of the API, and describes it
// for the log. It returns a nil handler when there is no UI to serve.
//
// A built web/dist wins: that is the development loop, where `make web_build`
// is visible without recompiling. The copy embedded at build time is the
// fallback, and it is what makes a released binary self-contained.
func pickWebUI(disk, embedded fs.FS, api http.Handler) (http.Handler, string) {
	if webui.HasIndex(disk) {
		return webui.Handler(disk, api), webDistFolder()
	}
	if webui.HasIndex(embedded) {
		return webui.Handler(embedded, api), "the copy embedded at build time"
	}
	return nil, ""
}
