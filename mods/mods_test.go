package mods

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExtract(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "irc-view")
	if err := Extract(IRCView, "irc-view", dir); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		".claude-plugin/plugin.json",
		"hooks/hooks.json",
		"hooks/register.tsx",
		"types/index.d.ts",
	} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Error(err)
		}
	}
}
