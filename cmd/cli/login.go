package main

import "fmt"

type LoginCmd struct {
	Args []string `arg:"" optional:"" help:"args for login"`
}

func (cmd *LoginCmd) Run() error {
	fmt.Println(cmd.Args)
	return nil
}
