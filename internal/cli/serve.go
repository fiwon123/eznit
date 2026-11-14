package cli

import "github.com/fiwon123/eznit/internal/server"

// ServeCmd runs the API in this binary. `eznit serve` is what the desktop
// window starts from a bundle, and what a server runs on a machine that other
// clients reach — the same program as the client, so the two cannot disagree
// about their version.
//
// It takes no flag of its own: `--debug` is the command tree's, and `eznit
// --debug serve` is how the server turns debug logging on. Run logs for itself
// (JSON on stdout, which the window reads as the server's log), so the client's
// console logger is created after it.
type ServeCmd struct{}

func (cmd *ServeCmd) Run(g *Globals) error {
	return server.Run(server.Options{Debug: g.debug})
}
