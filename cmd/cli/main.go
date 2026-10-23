// The CLI is a thin entrypoint: internal/cli holds the commands, the client
// and the configuration they read, so this file only names the version and
// hands over the arguments.
package main

import "github.com/fiwon123/eznit/internal/cli"

// Version is what `eznit -v` prints. It is set at build time:
//
//	go build -ldflags "-X main.Version=v1.2.3" ./cmd/cli
var Version = "dev"

func main() {
	cli.Run(Version)
}
