package main

type ListCmd struct {
	Args []string `arg:"" optional:"" help:"args for list"`
}
