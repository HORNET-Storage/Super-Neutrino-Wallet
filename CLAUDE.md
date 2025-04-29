# CLAUDE.md - Development Guide

## Build & Run Commands
- Build: `./build.sh` (Linux/macOS) or `.\build.ps1` (Windows)
- Run: `./SN-wallet`
- Tests: `go test ./...` or `go test ./path/to/package`
- Single test: `go test ./path/to/package -run TestName`
- Format code: `go fmt ./...`
- Lint: `golint ./...` (install with `go install golang.org/x/lint/golint@latest`)

## Code Style Guidelines
- **Imports**: Standard lib first, third-party next, local packages last
- **Error handling**: Return errors with context (`fmt.Errorf("context: %w", err)`)
- **Naming**: 
  - Functions/methods: PascalCase for exported, camelCase for private
  - Variables: camelCase
  - Packages: lowercase, single word
- **Types**: Define structs at package level with clear documentation
- **Comments**: Document all exported functions, types, and constants
- **Testing**: Write unit tests for all non-trivial functions
- **Logging**: Use internal logger package for structured logging
- **Concurrency**: Use channels and goroutines with proper context cancelation
- **Configuration**: Use internal/config package with sensible defaults

*This file serves as guidance for agentic coding assistants working in this repository.*