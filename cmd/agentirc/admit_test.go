package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestOKDM(t *testing.T) {
	for _, tt := range []struct {
		policy string
		ok     bool
	}{
		{dmAny, true},
		{dmMembers, true},
		{dmOperator, true},
		{"", false},
		{"friends", false},
	} {
		if err := okDM(tt.policy); (err == nil) != tt.ok {
			t.Errorf("okDM(%q) = %v, want ok %v", tt.policy, err, tt.ok)
		}
	}
}

// TestBridgeDropsPrivateMessagesFromStrangers sends a bridge that
// admits members and its operator a message from a node it shares no
// channel with and one from the operator. Only the operator's is filed.
func TestBridgeDropsPrivateMessagesFromStrangers(t *testing.T) {
	if testing.Short() {
		t.Skip("network test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	dir := t.TempDir()
	alice := testNode(t, ctx, "alice")
	bob := testNode(t, ctx, "bob")
	carol := testNode(t, ctx, "carol")

	b := newBridge(alice)
	b.dm, b.operator = dmMembers, carol.ID()
	toChat, toBridge := pipePair()
	b.attach(ctx, toChat)
	go serveChat(ctx, toBridge, dir, "alice", "")

	if err := bob.SendPrivate(ctx, alice.ID(), alice.Addr(), "hello from bob"); err != nil {
		t.Fatalf("private message from a stranger: %v", err)
	}
	if err := carol.SendPrivate(ctx, alice.ID(), alice.Addr(), "hello from carol"); err != nil {
		t.Fatalf("private message from the operator: %v", err)
	}
	waitFor(t, filepath.Join(dir, "carol", "out"), "hello from carol")
	if _, err := os.Stat(filepath.Join(dir, "bob")); err == nil {
		t.Error("a stranger's private message was filed")
	}
}
