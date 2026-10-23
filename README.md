# edit

A keyboard-driven text editor for the terminal, laid out like an IDE: a file
sidebar on the left and the open files as tabs on the right.

> **Status:** early. The layout is in place — header, file sidebar, tabbed
> editor pane and key-hint footer. The sidebar browses the workspace as a
> folder tree and opens files in tabs, where you edit them, with undo and
> redo, colored by their syntax; edits are saved on their own, in the
> background. Every key can be rebound, and the colors picked, in a config
> file.

Built with the [Charm](https://charm.sh) stack: [Bubble Tea](https://github.com/charmbracelet/bubbletea)
and [Lipgloss](https://github.com/charmbracelet/lipgloss), with [Cobra](https://github.com/spf13/cobra)
for the CLI.

## Usage

```sh
edit                                # opens the current directory as the workspace
edit --config path/to/config.yaml   # reads the settings from another file
```

These keys work in either pane:

| Key | Action | Does |
|-----|--------|------|
| `shift+tab` | `global.switch-focus` | Switch focus between the file sidebar and the editor |
| `ctrl+pgdown` / `ctrl+pgup` | `global.next-tab` / `global.previous-tab` | Switch to the next / previous tab, wrapping around |
| `ctrl+w` | `global.close-tab` | Save and close the active tab |
| `ctrl+q` / `ctrl+c` | `global.quit` | Save every tab and quit |

These, while the file sidebar has focus:

| Key | Action | Does |
|-----|--------|------|
| `↑` / `↓` | `sidebar.up` / `sidebar.down` | Move through the file tree |
| `pgup` / `pgdown` | `sidebar.page-up` / `sidebar.page-down` | Move a page up / down |
| `home` / `end` | `sidebar.top` / `sidebar.bottom` | Jump to the first / last entry |
| `→` | `sidebar.expand` | Expand a folder; on an open folder, step into it |
| `←` | `sidebar.collapse` | Collapse a folder; elsewhere, go to the parent folder |
| `enter` | `sidebar.open` | Open a file in a tab, or expand / collapse a folder |

And these, while the editor has focus:

| Key | Action | Does |
|-----|--------|------|
| `←` / `→` | `editor.left` / `editor.right` | Move the cursor a character, within the line |
| `↑` / `↓` | `editor.up` / `editor.down` | Move the cursor a line, keeping its column |
| `home` / `end` | `editor.line-start` / `editor.line-end` | Go to the start / end of the line |
| `pgup` / `pgdown` | `editor.page-up` / `editor.page-down` | Move a page up / down |
| `ctrl+home` / `ctrl+end` | `editor.top` / `editor.bottom` | Go to the start / end of the file |
| `enter` | `editor.newline` | Split the line at the cursor |
| `tab` | `editor.insert-tab` | Type a tab |
| `backspace`, `ctrl+h` | `editor.delete-left` | Delete the character before the cursor; at a line start, join the line to the one above |
| `delete` | `editor.delete-right` | Delete the character under the cursor; at a line end, join the next line to this one |
| `ctrl+k` | `editor.delete-line` | Delete the cursor's line |
| `ctrl+z` | `editor.undo` | Undo the last edit |
| `ctrl+shift+z`, `ctrl+y` | `editor.redo` | Redo the last edit undo took back |

Any other character key types itself, and pasted text goes in at the cursor.
ctrl+shift+z only reaches edit from a terminal that reports it — see
[Keys your terminal must report](#keys-your-terminal-must-report); where it
arrives as ctrl+z, it undoes, and ctrl+y redoes.
Every key can be rebound in the config file: see
[Configuration](#configuration). The tree lists hidden files too, and reads a
folder from disk each time it is expanded.

### Editing and saving

There is no save key. edit writes each tab's edits to disk in the background,
every 500 ms, so an edit is on disk about half a second after you make it.
Closing a tab saves it first, and quitting saves every tab — as does stopping
edit with SIGTERM or SIGINT, or closing its terminal. A file you only look at
is never written.

- **What is written** is the file as it was read, with your edits: every
  byte you didn't touch stays as it was. A file whose lines all end in CRLF
  keeps CRLF, on the lines you add too, and the final line break stays, or
  stays absent.
- **In place.** A file is overwritten where it is, so it keeps its
  permissions, owner and hard links, and a symlink keeps pointing at the
  file it names; no temporary file appears in the workspace.
- **When a write fails** — a read-only file, a full disk — the tab's title
  gets a `⚠` and the header says why; edit keeps retrying every 500 ms.
  Closing or quitting then stops and asks: the same key again closes or
  quits anyway, and the edits that couldn't be saved are lost; any other key
  cancels. After quitting anyway, edit prints what it couldn't save.
- **Undo** takes back one step at a time: a word you typed with the spaces,
  tabs or line breaks after it, a run of backspaces, a run of deletes, a
  paste, a deleted line. Moving the cursor ends a step. Redo brings back
  what undo took, until you edit again. Each tab keeps its own history, up
  to 1000 steps, for as long as it is open: closing it drops the history.
  An undo is saved like any edit.
- A file is read when its tab opens. Changes made to it on disk while it is
  open are not noticed, and the next save overwrites them.
- The cursor moves over, and deletes, characters as they are drawn: an
  accented letter or an emoji is one step. A tab is drawn as spaces up to
  the next multiple of 4 columns; control characters, each byte that is not
  valid UTF-8, and characters with no width of their own are drawn as `?`.
  Long lines aren't wrapped: the view scrolls sideways.
- Binary files and files over 10 MB open with a note instead of their text,
  and can't be edited.

### Syntax colors

edit colors the text of Go (and `go.mod`), YAML, JSON, Markdown, shell,
Makefile, JavaScript, Python and Java files by their syntax: keywords,
strings, numbers, comments, the names of functions and types where they are
declared and used, and, in Markdown, headings, emphasis, code and links, a
fenced code block in one of these languages colored as that language. A
file's language comes from its name — `main.go`, `Makefile`, `.bashrc` —
or, when its name says nothing, from a `#!` line naming `sh`, `bash`, `node`
or `python`. Other files are drawn without colors.

- The colors come from parsing the whole file in the background, so typing
  never waits for them. An edit shows at once, the colors after it moving
  along with their characters; the colors of what you typed follow when the
  parse lands, a few milliseconds later for most files, up to about a second
  for a large Markdown file.
- Files over 256 KB, and lines over 20,000 bytes, are drawn without colors.
  A parse that takes over 2 s stops, and the file keeps the colors it had.
- The `theme` setting picks the colors: see [Configuration](#configuration).

## Configuration

edit reads its settings from `config.yaml` in an `edit` folder of your config
directory: `~/.config/edit/config.yaml` on Linux (or under `$XDG_CONFIG_HOME`
when it is set), `~/Library/Application Support/edit/config.yaml` on macOS.
`--config` names another file. The file is optional: without it, edit uses
the keys above and the theme `candy`. edit reads it once, at start-up, and
never writes it.

It has two settings. `theme` picks the colors of the text:

```yaml
theme: dracula
```

| Theme | For |
|---|---|
| `candy` | dark terminals, in the 256-color palette of edit's own frame; the default |
| `candy-light` | light terminals |
| `dracula` | the [Dracula](https://draculatheme.com) palette |
| `gruvbox` | the [Gruvbox](https://github.com/morhetz/gruvbox) dark palette |
| `none` | no colors |

A theme sets the color of the text, bold, italics and underline, never the
background, so each works on your terminal's own. On a terminal without
colors, or with `NO_COLOR` set, edit draws none, whatever the theme.

`shortcuts` rebinds keys. It has three scopes: `global`, whose actions work
in either pane, `sidebar` and `editor`. Each scope maps action names, from
the tables above, to a key or a list of keys:

```yaml
shortcuts:
  global:
    next-tab: alt+right        # a single key needs no brackets
    previous-tab: alt+left
    switch-focus: [shift+tab, f6]
  sidebar:
    down: [down, j]            # the first key is the one the footer shows
    up: [up, k]
    collapse: [left, h]
    expand: [right, l]
  editor:
    line-start: [home, ctrl+a]
    line-end: [end, ctrl+e]
```

- Listing an action replaces all of its default keys; the actions you don't
  list keep theirs. So `close-tab: ctrl+x` frees `ctrl+w`.
- `[]` unbinds an action. `quit` must keep at least one key.
- A key does one thing wherever it works: two actions of one scope can't
  share a key, and neither can a `global` action and a pane's. The same key in
  `sidebar` and `editor` is fine, since only one pane has focus.
- Keys that type text — a single character, or `space` — can't be bound in
  `editor`, where they type, nor in `global`, whose actions work in the
  editor too. `alt+` and `ctrl+` keys are fine there, and letters are fine in
  `sidebar`.
- The first key of an action is the one the footer shows.
- A YAML anchor lets scopes share a list: `down: &down [down, j]` in
  `sidebar`, then `down: *down` in `editor`.

Keys are written as [Bubble Tea](https://github.com/charmbracelet/bubbletea)
names them, which is also how the footer shows them: any modifiers, then
the key.

| Kind | Names |
|---|---|
| Characters | `a`, `J` (that is shift+j), `?`, `1`, `ñ`, … — any single character |
| Space bar | `space` |
| Editing keys | `tab`, `enter`, `esc`, `backspace`, `delete`, `insert` |
| Arrows and paging | `up`, `down`, `left`, `right`, `home`, `end`, `pgup`, `pgdown` |
| Function keys | `f1` … `f63` |
| Modifiers | `ctrl+`, `alt+`, `shift+`, `meta+`, `hyper+` or `super+` in front of any of the above, alone or together: `ctrl+w`, `alt+right`, `ctrl+shift+z`, `shift+enter`, `alt+space` |

Names are lower-case but for the character itself, and the modifiers can
come in any order: edit writes them ctrl, alt, shift, meta, hyper, super,
and a capital after a modifier as shift and the small letter, so
`shift+ctrl+Z` is `ctrl+shift+z` and `alt+J` is `alt+shift+j`, as the footer
shows them. `shift+j` is not a key (write `J`), and neither is shift before
a sign: write the sign shift types, `alt+?` rather than `alt+shift+/`. With
ctrl, a letter of a script other than Latin is the key it is on in the US
layout, as terminals send it: on a Russian layout, ctrl+й is `ctrl+q`.
ctrl+space is `ctrl+space`;
`ctrl+@`, the name older versions of edit gave it, still means it. Keys edit
never receives are not accepted: a modifier or lock key pressed alone, such
as `leftshift` or `capslock`. A key that means something in YAML — `#`, `:`,
`[`, `]`, `{`, `}`, `,`, `&`, `*`, `!`, `|`, `>`, `'`, `"`, `%`, `@` or
`` ` `` — must be quoted: `"#"`.

### Keys your terminal must report

Some keys reach edit only from a terminal that reports them: ctrl+shift with
a letter, shift+enter, ctrl+i apart from tab, ctrl+m apart from enter, and
the like. Terminals with the kitty keyboard protocol — kitty, Ghostty, foot,
Alacritty, WezTerm with `enable_kitty_keyboard` — or with xterm's
modifyOtherKeys report them. Others send the key without its shift, or as
the key it shares a byte with: ctrl+shift+z arrives as ctrl+z, ctrl+i as
tab. Under tmux they need `set -g extended-keys on` in its config.

Konsole reports none of them, but a key binding in its profile can send one
itself: in Settings → Edit Current Profile → Keyboard, edit the key bindings
and add a row with the key combination `Z+Ctrl+Shift` and the output
`\E[122;6u` — the letter's code (122 for z, 97 for a), then `;6u`. kitty,
foot and WezTerm use ctrl+shift+z for shortcuts of their own until you
unbind it there.

A mistake in the file stops edit before it starts, with a message that names
the file and lists every problem:

```
config /home/you/.config/edit/config.yaml: invalid shortcuts:
  editor.down: key "j" types text
  editor.up: unknown key "ctrl+uo"
  key "j" is bound to both sidebar.down and sidebar.up
```

or, for a theme edit doesn't have:

```
config /home/you/.config/edit/config.yaml: unknown theme "monokai": want candy, candy-light, dracula, gruvbox or none
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

This binary embeds the grammars of every language the syntax-coloring
library knows, about 32 MB; the release binaries embed only edit's ten,
about 14 MB.

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

Packages under `libs/` (`libs/treeview`, `libs/tabset`, `libs/textedit`,
`libs/keymap`, `libs/syntax` and `libs/syntax/treesitter`) are app-agnostic:
they must never import `app/` or `internal/`. A depguard rule in
`.golangci.yaml` enforces this, so `make lint` fails on a wrong-way import.

### Syntax colors

`libs/syntax` is the plug-in boundary: an engine implements its `Language`
and `Tree`, and its `Highlighter` keeps one text's tree up to date in the
background. `libs/syntax/treesitter` is the engine edit uses: tree-sitter
grammars run in pure Go by
[gotreesitter](https://github.com/odvcencio/gotreesitter), with edit's own
highlight queries in `libs/syntax/treesitter/queries/`.

gotreesitter's build tags pick the grammars a build embeds. The Makefile's
`GRAMMARS` lists edit's, and every `make` target passes them to `go`, as
`.golangci.yaml` and `.goreleaser.yaml` do; a plain `go build` or `go test`
works too, embedding all 206. To color another language:

1. add its grammar's tag to `GRAMMARS`, `.golangci.yaml` and `.goreleaser.yaml`;
2. write `libs/syntax/treesitter/queries/<grammar>/highlights.scm`, capturing
   only the names `libs/syntax/captures.go` lists;
3. add the language to the list in `libs/syntax/treesitter/treesitter.go`,
   and a text of it to the engine's tests.

The engine's tests fail on a grammar missing from the build, a query that
doesn't compile, or a capture name outside the list.

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
