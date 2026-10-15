package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/candy-tools/edit/app/metainfo"
)

func TestPrintVersion(t *testing.T) {
	var buf bytes.Buffer
	printVersion(&buf)
	out := buf.String()
	if !strings.Contains(out, metainfo.Version) {
		t.Errorf("output should contain the version %q:\n%s", metainfo.Version, out)
	}
	for _, want := range []string{"commit:", "built:", "compiler:"} {
		if !strings.Contains(out, want) {
			t.Errorf("version output missing %q", want)
		}
	}
}

func TestVersionCommandRuns(t *testing.T) {
	c := newRootCommand()
	var buf bytes.Buffer
	c.SetOut(&buf)
	c.SetArgs([]string{"version"})
	if err := c.Execute(); err != nil {
		t.Fatalf("version command failed: %v", err)
	}
	if !strings.Contains(buf.String(), "edit ") {
		t.Errorf("unexpected version output:\n%s", buf.String())
	}
}
