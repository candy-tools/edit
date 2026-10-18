# edit

A keyboard-driven text editor for the terminal, laid out like an IDE: a file
sidebar on the left and the open files as tabs on the right.

> **Status:** early bootstrap. The layout is in place — header, file sidebar,
> tabbed editor pane and key-hint footer — and the sidebar browses the
> workspace as a folder tree, but nothing can be opened or edited yet.

Built with the [Charm](https://charm.sh) stack: [Bubble Tea](https://github.com/charmbracelet/bubbletea)
and [Lipgloss](https://github.com/charmbracelet/lipgloss), with [Cobra](https://github.com/spf13/cobra)
for the CLI.

## Usage

```sh
edit    # opens the current directory as the workspace
```

| Key | Action |
|-----|--------|
| `tab` | Switch focus between the file sidebar and the editor |
| `↑` / `↓` | Move through the file tree |
| `pgup` / `pgdown` | Move a page up / down |
| `home` / `end` | Jump to the first / last entry |
| `→` | Expand a folder; on an open folder, step into it |
| `←` | Collapse a folder; elsewhere, go to the parent folder |
| `enter` | Expand or collapse a folder |
| `ctrl+q` / `ctrl+c` | Quit |

The file tree keys work while the sidebar has focus. The tree lists hidden
files too, and reads a folder from disk each time it is expanded.

## Install

edit is a single, dependency-free binary (pure Go, no CGO). Prebuilt archives, a
Debian package, and a macOS cask are published on every tagged release.

### macOS (Homebrew)

A macOS cask is published to the
[candy-tools/homebrew-tap](https://github.com/candy-tools/homebrew-tap) tap on
every tagged release. Add the tap, then install:

```bash
brew tap candy-tools/tap
brew install --cask edit
```

`brew upgrade` tracks future releases. The binary isn't notarized, so the cask
strips the Gatekeeper quarantine flag on install — no "edit is damaged" prompt.

### Debian / Ubuntu

Download the `.deb` for your architecture from the
[releases page](https://github.com/candy-tools/edit/releases) and install it:

```bash
sudo apt install ./edit_*_amd64.deb
```

### From source

With a Go 1.26+ toolchain installed:

```bash
go install github.com/candy-tools/edit@latest
```

Or grab a prebuilt `tar.gz` (`.zip` on Windows) for your OS/arch from the
[releases page](https://github.com/candy-tools/edit/releases).

## Develop

Requires Go 1.26+. The full toolchain also needs `golangci-lint`, `goreleaser`,
and `go-licence-detector`. edit has no runtime dependencies — it's a single
static binary.

```bash
make help         # list all targets
make test         # run tests with coverage
make lint         # golangci-lint
make vet          # go vet
make coverage     # enforce the per-package coverage threshold
make verify       # all of the above plus the license check
```

Packages under `libs/` (such as `libs/treeview`) are app-agnostic: they must
never import `app/` or `internal/`. A depguard rule in `.golangci.yaml`
enforces this, so `make lint` fails on a wrong-way import.

### Run

```bash
make run          # runs the editor on the current directory
# or plain go:
go run .
```

### Build

```bash
make build        # goreleaser snapshot build for the current OS/arch → ./dist
# or plain go:
go build ./...
```

### Release

Releases are published by pushing a semver tag from a clean `main` branch. The
tag triggers the [Release workflow](./.github/workflows/release.yml), which runs
GoReleaser to build and publish the archives, `.deb`, and Homebrew cask.

```bash
make tag version="v1.2.3"
```

`make tag` refuses to run unless you're on `main` with a clean working tree, then
creates and pushes the `vX.Y.Z` tag.
