package cli

import (
	"log"
	"os"

	"github.com/alecthomas/kong"
	"github.com/fiwon123/eznit/pkg/logger"
	"github.com/joho/godotenv"
)

type API struct {
	baseURL string
	host    string
	port    string
}

func newAPI(host string, port string) *API {
	return &API{
		baseURL: host + ":" + port,
		host:    host,
		port:    port,
	}
}

type Globals struct {
	api       *API
	downloads string
	logger    *logger.Config
}

func newGlobals(api *API, downloads string, logger *logger.Config) *Globals {
	return &Globals{
		api:       api,
		downloads: downloads,
		logger:    logger,
	}
}

type CLI struct {
	Debug   bool             `help:"enable debug level"`
	Version kong.VersionFlag `short:"v" help:"show version."`

	Login    LoginCmd    `cmd:"" aliases:"l" help:"save user credential"`
	Signup   SignupCmd   `cmd:"" aliases:"s" help:"create new user"`
	Download DownloadCmd `cmd:"" aliases:"d" help:"download a file"`
	Upload   UploadCmd   `cmd:"" aliases:"u" help:"upload a file"`
	List     ListCmd     `cmd:"" help:"list files"`
	Delete   DeleteCmd   `cmd:"" help:"delete file"`
	Share    ShareCmd    `cmd:"" help:"share a file"`
	Unshare  UnshareCmd  `cmd:"" help:"unshare a file"`
	Logout   LogoutCmd   `cmd:"" help:"logout user and remove credential"`

	Template TemplateCmd `cmd:"" help:"work with templates"`
	Command  CommandCmd  `cmd:"" help:"keep the commands your templates pull in"`
	Repo     RepoCmd     `cmd:"" help:"work with git hosts"`
}

var globals Globals

// Run is the CLI: it parses the arguments, builds the client and runs the
// command. version is what -v prints; cmd/cli sets it at build time.
func Run(version string) {

	// .env.local overwrite .env for development
	_ = godotenv.Load(".env.local", ".env")

	if len(os.Args) < 2 {
		os.Args = append(os.Args, "--help")
	}

	cli := CLI{}
	ctx := kong.Parse(&cli, kong.Vars{"version": version})

	logsFolder, _ := os.LookupEnv("CLI_LOGS")
	if logsFolder == "" {
		logsFolder = "./logs/"
	}
	l, err := logger.NewConsole(logsFolder, cli.Debug, false)
	if err != nil {
		log.Fatal(err)
	}
	defer l.Sync()

	host, _ := os.LookupEnv("CLI_API_HOST")
	if host == "" {
		host = "http://localhost"
	}

	port, _ := os.LookupEnv("CLI_API_PORT")
	if port == "" {
		port = "4000"
	}

	downloads, _ := os.LookupEnv("CLI_DOWNLOADS")
	if downloads == "" {
		downloads = "./downloads/"
	}

	api := newAPI(host, port)
	globals = *newGlobals(api, downloads, l)

	ctx.Run(&globals)
}

// getToken is the session token the API accepts. It lives in the CLI's own
// config file and is never sent anywhere but the configured API.
func getToken() (string, error) {
	return configValue("token")
}
