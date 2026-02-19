package main

import (
	"github.com/alecthomas/kong"
)

type CLI struct {
	Login  LoginCmd  `cmd:"" aliases:"l" help:"save user credential"`
	Signup SignupCmd `cmd:"" aliases:"s" help:"create new user"`
}

func main() {
	cli := CLI{}
	ctx := kong.Parse(&cli)
	err := ctx.Run()
	ctx.FatalIfErrorf(err)
}
