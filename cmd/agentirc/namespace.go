package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"fmt"
	"strings"

	"github.com/tmc/go-iroh-experiments/irciroh"
)

// A namespace is a secret that every channel a node joins by name is
// derived from. Two nodes meet in #agents only if they hold the same
// namespace: with different ones, or with none, the same name is a
// different gossip topic found under a different rendezvous key, and
// neither node can find or read the other's.
//
// It hides a channel from those without the namespace. It does not
// encrypt what is said to those within it; a sealed channel does that.

// namespaceSecret returns the channel secret for channel in namespace ns.
func namespaceSecret(ns, channel string) []byte {
	sum := sha256.Sum256([]byte("irciroh/namespace/v1\n" + ns + "\n" + irciroh.Fold(channel)))
	return sum[:]
}

// runNamespace prints a new namespace.
func runNamespace(args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("usage: agentirc namespace")
	}
	var b [20]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Errorf("namespace: %w", err)
	}
	fmt.Println(strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b[:])))
	return nil
}
