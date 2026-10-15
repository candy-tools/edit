package cmd

import (
	"errors"
	"os"
	"testing"
)

// stubTUI swaps the TUI launcher for fn for the duration of the test.
func stubTUI(t *testing.T, fn func(root string) error) {
	t.Helper()
	orig := runTUI
	runTUI = fn
	t.Cleanup(func() { runTUI = orig })
}

func TestRootArgs(t *testing.T) {
	c := newRootCommand()
	if err := c.Args(c, nil); err != nil {
		t.Errorf("no argument should be valid (opens the current directory): %v", err)
	}
	if err := c.Args(c, []string{"a.txt"}); err == nil {
		t.Error("expected an error with an argument")
	}
}

func TestRootMeta(t *testing.T) {
	c := newRootCommand()
	if c.Use == "" || c.Short == "" {
		t.Error("the root command should set Use and Short")
	}
}

func TestRootOpensWorkingDir(t *testing.T) {
	var got string
	stubTUI(t, func(root string) error {
		got = root
		return nil
	})
	c := newRootCommand()
	c.SetArgs([]string{})
	if err := c.Execute(); err != nil {
		t.Fatalf("root command failed: %v", err)
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if got != wd {
		t.Errorf("TUI started on %q, want the working directory %q", got, wd)
	}
}

func TestRootReturnsTUIError(t *testing.T) {
	boom := errors.New("boom")
	stubTUI(t, func(string) error { return boom })
	c := newRootCommand()
	c.SetArgs([]string{})
	if err := c.Execute(); !errors.Is(err, boom) {
		t.Errorf("Execute() = %v, want the TUI's error", err)
	}
}
