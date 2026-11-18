# Contributing

Thanks for your interest in contributing to `netportid`.

## Development setup

- A Linux environment is recommended (capture uses `AF_PACKET` and process mapping uses `/proc`)
- Go installed (see `go.mod` for the minimum supported version)
- Root privileges may be required to run/test live capture features

## Getting started

```bash
git clone https://github.com/jasonpiedrasantasdk/netportid.git
cd netportid
go mod tidy
go build ./...
```

## Before submitting changes

- Format code: `gofmt -w .`
- Run basic checks: `go vet ./...` and `go build ./...`
- Keep changes focused and describe user-facing behavior/impact in the PR

## Reporting issues

Please include:

- OS/distribution, kernel, and architecture
- Go version (`go version`)
- Command/flags used and whether running with required privileges
- Expected vs actual behavior and relevant logs/output

## Security & privacy

Avoid sharing logs or data that contain sensitive network or local process information.
