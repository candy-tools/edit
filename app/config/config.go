// Package config reads trazo's settings from its config file, a YAML file in
// the user's config directory.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"go.yaml.in/yaml/v3"
)

// Config is trazo's settings. The zero Config means the defaults.
type Config struct {
	// Shortcuts rebinds actions: scope → action name → keys. It holds what
	// the file says; which scopes, actions and keys exist is for the keymap
	// to check.
	Shortcuts map[string]map[string][]string
	// Theme names the color theme, "" for the default. Which themes exist
	// is for the editor to check.
	Theme string
	// Mouse turns the mouse on: off, the terminal keeps the mouse to
	// itself, to select text by dragging.
	Mouse bool
}

// DefaultPath is where trazo looks for its config file:
// <os.UserConfigDir>/trazo/config.yaml. It fails when there is no config
// directory, such as when $HOME is unset.
func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "trazo", "config.yaml"), nil
}

// Load reads the config file at path: one YAML document, a mapping of
// settings, which are shortcuts, mapping scopes to mappings of action names
// to a key or a list of keys, theme, a theme's name, and mouse, true or
// false. An empty file, and
// a null in place of a mapping, mean no settings there. A YAML alias stands
// for the value its anchor marks.
//
// Its errors don't name the file, since the caller knows it: a file that
// can't be read gives just the cause, such as "permission denied", and a
// missing file's error satisfies errors.Is(err, fs.ErrNotExist).
func Load(path string) (Config, error) {
	data, err := os.ReadFile(path) //nolint:gosec // G304: the path is the user's own config file, which trazo only reads
	if err != nil {
		var pe *fs.PathError
		if errors.As(err, &pe) {
			return Config{}, pe.Err
		}
		return Config{}, err
	}
	root, err := document(data)
	if err != nil {
		return Config{}, err
	}
	if root == nil {
		return Config{}, nil // an empty file, or one holding only comments
	}
	return settings(root)
}

// document returns the root of the one YAML document in data, or nil when
// data holds none, such as a file of only comments. Empty documents don't
// count, but a second one that holds anything is an error: trazo reads one,
// and would ignore the rest without a word.
func document(data []byte) (*yaml.Node, error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	var root *yaml.Node
	for {
		var doc yaml.Node
		err := dec.Decode(&doc)
		if errors.Is(err, io.EOF) {
			return root, nil
		}
		if err != nil {
			return nil, err
		}
		if len(doc.Content) == 0 || isNull(doc.Content[0]) {
			continue // an empty document, such as after a final ---
		}
		if root != nil {
			return nil, errors.New("want one document")
		}
		root = doc.Content[0]
	}
}

// settings reads the document: a mapping of settings.
func settings(n *yaml.Node) (Config, error) {
	var cfg Config
	err := mapping(n, "", "settings", func(name, at string, value *yaml.Node) error {
		switch name {
		case "shortcuts":
			var err error
			cfg.Shortcuts, err = shortcuts(value, at)
			return err
		case "theme":
			v := resolve(value)
			if !isKey(v) {
				return problem(at, "want a theme name")
			}
			cfg.Theme = v.Value
			return nil
		case "mouse":
			v := resolve(value)
			if v.Kind != yaml.ScalarNode || v.ShortTag() != "!!bool" {
				return problem(at, "want true or false")
			}
			return v.Decode(&cfg.Mouse)
		}
		return fmt.Errorf("unknown setting %q", name)
	})
	if err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// shortcuts reads the shortcuts setting, which is at path in the file:
// scope → action name → keys.
func shortcuts(n *yaml.Node, path string) (map[string]map[string][]string, error) {
	var out map[string]map[string][]string
	err := mapping(n, path, "scopes", func(scope, scopeAt string, actions *yaml.Node) error {
		bound := map[string][]string{}
		if out == nil {
			out = map[string]map[string][]string{}
		}
		out[scope] = bound
		return mapping(actions, scopeAt, "actions", func(name, at string, value *yaml.Node) error {
			keys, ok := keyList(value)
			if !ok {
				return problem(at, "want a key or a list of keys")
			}
			bound[name] = keys
			return nil
		})
	})
	return out, err
}

// mapping calls f with the name, the path in the file and the value of each
// entry of the mapping n, which is at path ("" for the whole document), in
// order; a null n is an empty mapping. what names the values the entries
// hold, for the error when n isn't a mapping of names. A name listed twice
// is an error too.
func mapping(n *yaml.Node, path, what string, f func(name, at string, value *yaml.Node) error) error {
	n = resolve(n)
	if isNull(n) {
		return nil
	}
	if n.Kind != yaml.MappingNode {
		return problem(path, "want a mapping of "+what)
	}
	seen := make(map[string]bool, len(n.Content)/2)
	for i := 0; i+1 < len(n.Content); i += 2 {
		key := resolve(n.Content[i])
		if key.Kind != yaml.ScalarNode {
			return problem(path, "want a mapping of "+what)
		}
		at := key.Value
		if path != "" {
			at = path + "." + key.Value
		}
		if seen[key.Value] {
			return problem(at, "listed twice")
		}
		seen[key.Value] = true
		if err := f(key.Value, at, n.Content[i+1]); err != nil {
			return err
		}
	}
	return nil
}

// problem is an error about the value at path in the file: "<path>: msg",
// or just msg for the whole document, at path "".
func problem(path, msg string) error {
	if path == "" {
		return errors.New(msg)
	}
	return errors.New(path + ": " + msg)
}

// keyList reads an action's keys: one key, or a list of keys. A key is a
// scalar other than null, read as written, so 1 is the key "1".
func keyList(n *yaml.Node) ([]string, bool) {
	n = resolve(n)
	if isKey(n) {
		return []string{n.Value}, true
	}
	if n.Kind != yaml.SequenceNode {
		return nil, false
	}
	keys := make([]string, 0, len(n.Content))
	for _, item := range n.Content {
		item = resolve(item)
		if !isKey(item) {
			return nil, false
		}
		keys = append(keys, item.Value)
	}
	return keys, true
}

// resolve follows an alias to the value its anchor marks.
func resolve(n *yaml.Node) *yaml.Node {
	for n.Kind == yaml.AliasNode {
		n = n.Alias
	}
	return n
}

// isNull reports whether n is a null: ~, null, or no value at all.
func isNull(n *yaml.Node) bool {
	return n.Kind == yaml.ScalarNode && n.ShortTag() == "!!null"
}

// isKey reports whether n can be a key: a scalar other than null.
func isKey(n *yaml.Node) bool {
	return n.Kind == yaml.ScalarNode && !isNull(n)
}
