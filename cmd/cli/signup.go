package main

import (
	"fmt"
	"os"
	"os/signal"

	"golang.org/x/term"
)

type SignupCmd struct {
	Args []string `arg:"" optional:"" help:"args for signup"`
}

func (cmd *SignupCmd) Run() error {
	var email string

	fmt.Print("Enter Email: ")
	fmt.Scan(&email)
	password, err := promptPassword("Enter Password: ")
	if err != nil {
		return err
	}

	confirm, err := promptPassword("Confirm password: ")
	if err != nil {
		return err
	}

	if string(password) != string(confirm) {
		return fmt.Errorf("passwords do not match")
	}

	fmt.Println("Account created successfully!")
	return nil
}

func promptPassword(msg string) (string, error) {
	fd := int(os.Stdin.Fd())

	oldState, err := term.GetState(fd)
	if err != nil {
		return "", err
	}
	defer term.Restore(fd, oldState)

	sigChan := make(chan os.Signal, 1)
	doneChan := make(chan struct{})

	signal.Notify(sigChan, os.Interrupt)

	go func() {
		select {
		case <-sigChan:
			term.Restore(fd, oldState)
			fmt.Println("\nOperation cancelled.")
			os.Exit(1)
		case <-doneChan:
			return
		}
	}()

	defer func() {
		signal.Stop(sigChan)
		close(doneChan)
	}()

	fmt.Print(msg)

	bytePass, err := term.ReadPassword(fd)
	fmt.Println()
	if err != nil {
		return "", err
	}
	return string(bytePass), nil
}
