package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/fiwon123/eznit/internal/domain/users"
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

	err = sendRequest(users.CreateRequest{
		Email:    email,
		Password: password,
	})
	if err != nil {
		return fmt.Errorf("internal server error")
	}

	fmt.Println("Account created successfully!")
	return nil
}

func sendRequest(request users.CreateRequest) error {
	jsonData, err := json.Marshal(request)
	if err != nil {
		return err
	}

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := client.Post("http://localhost:4000/v1/users", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned error: %s", resp.Status)
	}

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
