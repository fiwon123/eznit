package main

import (
	"fmt"

	"github.com/alecthomas/kong"
)

type LoginCmd struct {
	Paths []string `arg:"" optional:"" help:"Paths to Login."`
}

type SignupCmd struct {
	Paths []string `arg:"" optional:"" help:"Paths to Signup."`
}

type CLI struct {
	Login  LoginCmd  `cmd:"" aliases:"l" help:"save user credential"`
	Signup SignupCmd `cmd:"" aliases:"s" help:"create new user"`
}

func main() {
	cli := CLI{}
	ctx := kong.Parse(&cli)
	switch ctx.Selected().Name {
	case "login":
		fmt.Println("Login", cli.Login.Paths)
	case "signup":
		fmt.Println("SignUp", cli.Signup.Paths)
	default:
		panic(ctx.Command())
	}
}
