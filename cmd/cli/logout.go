package main

import "fmt"

type LogoutCmd struct {
}

func (cmd *LogoutCmd) Run(g *Globals) error {
	fmt.Println("logout")

	return nil
}
