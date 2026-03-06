package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/fiwon123/eznit/internal/domain/users"
	"github.com/fiwon123/eznit/pkg/types"
	"golang.org/x/term"
)

type SignupCmd struct {
}

func (cmd *SignupCmd) Run(g *Globals) error {
	fmt.Println("signup")

	var email string

	fmt.Print("Enter Email: ")
	fmt.Scan(&email)
	password, err := promptPassword("Enter Password: ")
	if err != nil {
		g.logger.Warn("prompt password. ", slog.String("error", err.Error()))
		return nil
	}

	confirm, err := promptPassword("Confirm password: ")
	if err != nil {
		g.logger.Warn("prompt confirm password. ", slog.String("error", err.Error()))
		return nil
	}

	fmt.Println("")
	if string(password) != string(confirm) {
		g.logger.Warn("passwords do not match. ")
		return nil
	}

	ok := sendSignupRequest(g.api.baseURL, users.SignupRequest{
		Email:           email,
		Password:        password,
		ConfirmPassword: confirm,
	}, g)
	if !ok {
		return nil
	}

	g.logger.Info("Account created successfully! ")
	return nil
}

func sendSignupRequest(baseURL string, request users.SignupRequest, g *Globals) bool {
	jsonData, err := json.Marshal(request)
	if err != nil {
		g.logger.Error("failed to convert request to json. ", slog.Any("error", err))
		return false
	}

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := client.Post(baseURL+"/v1/signup", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		g.logger.Warn("request failed. ", slog.String("error", err.Error()))
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		var response types.Envelope[any]

		if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
			g.logger.Error("failed to decode. ", slog.String("error", err.Error()))
			return false
		}

		g.logger.Warn("signup failed. ", slog.Any("result", response))

		return false
	}

	return true
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
