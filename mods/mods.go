// Package mods holds the client mods that agentirc launch can install.
package mods

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// IRCView is a Claude Code mod that draws the irciroh tool calls and
// channel events as IRC lines. Its root is the directory irc-view.
//
//go:embed all:irc-view
var IRCView embed.FS

// Extract writes the tree at root in fsys to dir, replacing files
// already there.
func Extract(fsys fs.FS, root, dir string) error {
	return fs.WalkDir(fsys, root, func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, filepath.FromSlash(name))
		if err != nil {
			return err
		}
		dst := filepath.Join(dir, rel)
		if d.IsDir() {
			return os.MkdirAll(dst, 0o700)
		}
		b, err := fs.ReadFile(fsys, name)
		if err != nil {
			return err
		}
		if err := os.WriteFile(dst, b, 0o600); err != nil {
			return fmt.Errorf("extract %s: %w", name, err)
		}
		return nil
	})
}
