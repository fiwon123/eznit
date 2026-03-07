# eznit

Eznit is a tool to manage text based files (.txt, .cfg, .json, .yaml, ...) using a local storage space. Available operations: search, publish, download, preview, etc...

## OS Support

- Windows
- Linux

## About

This project has two entry points API Server and CLI Tool:

### API Server

API server is responsible to manage all incoming request from Eznit CLI Tool.

- type `go run ./cmd/api` to start api server

### CLI Tool

CLI tool is responsible to handle all command by user to manage your internal files

- type `go run ./cmd/cli --help` to show availables parameters
- cmds: signup, login, logout, upload, download, list, delete
