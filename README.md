# edit

A keyboard-driven text editor for the terminal, laid out like an IDE: a file
sidebar on the left and the open files as tabs on the right.

> **Status:** early bootstrap. The layout is in place — header, file sidebar,
> tabbed editor pane and key-hint footer. The sidebar browses the workspace
> as a folder tree and opens files, read-only, in tabs; nothing can be edited
> yet. Every key can be rebound in a config file.

Built with the [Charm](https://charm.sh) stack: [Bubble Tea](https://github.com/charmbracelet/bubbletea)
and [Lipgloss](https://github.com/charmbracelet/lipgloss), with [Cobra](https://github.com/spf13/cobra)
for the CLI.

## Usage

```sh
edit                                # opens the current directory as the workspace
edit --config path/to/config.yaml   # reads the settings from another file
```

| Key | Action | Does |
|-----|--------|------|
| `tab` | `global.switch-focus` | Switch focus between the file sidebar and the editor |
| `↑` / `↓` | `sidebar.up` / `sidebar.down`, `editor.up` / `editor.down` | Move through the file tree, or scroll the open file a line |
| `pgup` / `pgdown` | `sidebar.page-up` / `sidebar.page-down`, `editor.page-up` / `editor.page-down` | Move or scroll a page up / down |
| `home` / `end` | `sidebar.top` / `sidebar.bottom`, `editor.top` / `editor.bottom` | Jump to the first / last entry, or the top / bottom of the file |
| `→` | `sidebar.expand` | Expand a folder; on an open folder, step into it |
| `←` | `sidebar.collapse` | Collapse a folder; elsewhere, go to the parent folder |
| `enter` | `sidebar.open` | Open a file in a tab, or expand / collapse a folder |
| `ctrl+pgdown` / `ctrl+pgup` | `global.next-tab` / `global.previous-tab` | Switch to the next / previous tab, wrapping around |
| `ctrl+w` | `global.close-tab` | Close the active tab |
| `ctrl+q` / `ctrl+c` | `global.quit` | Quit |

The file tree keys work while the sidebar has focus, and the scroll keys while
the editor has focus; the tab keys work in either. Every key can be rebound
in the config file: see [Configuration](#configuration). The tree lists hidden
files too, and reads a folder from disk each time it is expanded. A file is
read when its tab opens: to see later changes on disk, close the tab and open
the file again.

## Configuration

edit reads its settings from `config.yaml` in an `edit` folder of your config
directory: `~/.config/edit/config.yaml` on Linux (or under `$XDG_CONFIG_HOME`
when it is set), `~/Library/Application Support/edit/config.yaml` on macOS.
`--config` names another file. The file is optional: without it, edit uses
the keys above. edit reads it once, at start-up, and never writes it.

Its one setting, `shortcuts`, rebinds keys. It has three scopes: `global`,
whose actions work in either pane, `sidebar` and `editor`. Each scope maps
action names, from the table above, to a key or a list of keys:

```yaml
shortcuts:
  global:
    next-tab: alt+right        # a single key needs no brackets
    previous-tab: alt+left
  sidebar:
    down: [down, j]
    up: [up, k]
    collapse: [left, h]
    expand: [right, l]
  editor:
    down: [j, down]            # the first key is the one the footer shows
    up: [k, up]
```

- Listing an action replaces all of its default keys; the actions you don't
  list keep theirs. So `close-tab: ctrl+x` frees `ctrl+w`.
- `[]` unbinds an action. `quit` must keep at least one key.
- A key does one thing wherever it works: two actions of one scope can't
  share a key, and neither can a `global` action and a pane's. The same key in
  `sidebar` and `editor` is fine, since only one pane has focus.
- The first key of an action is the one the footer shows.
- A YAML anchor lets scopes share a list: `down: &down [down, j]` in
  `sidebar`, then `down: *down` in `editor`.

Keys are written as [Bubble Tea](https://github.com/charmbracelet/bubbletea)
names them, which is also how the footer shows them:

| Kind | Names |
|---|---|
| Characters | `a`, `J` (that is shift+j), `?`, `1`, `ñ`, … — any single character |
| Space bar | `space` |
| Control keys | `ctrl+a` … `ctrl+z` but `ctrl+i` and `ctrl+m` (those are `tab` and `enter`), `ctrl+@`, `ctrl+\`, `ctrl+]`, `ctrl+^`, `ctrl+_` |
| Editing keys | `tab`, `shift+tab`, `enter`, `esc`, `backspace`, `delete`, `insert` |
| Arrows | `up`, `down`, `left`, `right`, with `shift+`, `ctrl+` or `ctrl+shift+` in front |
| Paging | `home`, `end`, `pgup`, `pgdown`, `ctrl+pgup`, `ctrl+pgdown`, and `home` / `end` with `shift+`, `ctrl+` or `ctrl+shift+` |
| Function keys | `f1` … `f20` |
| Alt | `alt+` in front of any of the above: `alt+x`, `alt+right`, `alt+space` |

Names are lower-case, and only keys a terminal can send are accepted:
`ctrl+shift+w` is not a key, and neither is `shift+j` (write `J`). Most
terminals send `ctrl+@` for ctrl+space. A key that means something in YAML —
`#`, `:`, `[`, `]`, `{`, `}`, `,`, `&`, `*`, `!`, `|`, `>`, `'`, `"`, `%`,
`@` or `` ` `` — must be quoted: `"#"`.

A mistake in the file stops edit before it starts, with a message that names
the file and lists every problem:

```
config /home/you/.config/edit/config.yaml: invalid shortcuts:
  editor.up: unknown key "ctrl+uo"
  key "j" is bound to both sidebar.down and sidebar.up
```

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

Packages under `libs/` (`libs/treeview`, `libs/tabset`, `libs/textview` and
`libs/keymap`) are app-agnostic: they must never import `app/` or
`internal/`. A depguard rule in `.golangci.yaml` enforces this, so
`make lint` fails on a wrong-way import.

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
