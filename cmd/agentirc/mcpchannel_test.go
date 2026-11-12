package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestChannelEvent(t *testing.T) {
	self := strings.Repeat("ab", 32)
	other := strings.Repeat("cd", 32)
	const when = "2026-11-11T17:00:00Z"
	tests := []struct {
		name string
		line Line
		want *channelParams
	}{
		{
			name: "message",
			line: Line{Time: when, Target: "#go", Kind: "message", From: "alice", Key: other, Verdict: "unknown", Text: "hi"},
			want: &channelParams{Content: "hi", Meta: map[string]string{
				"target": "#go", "from": "alice", "key": other, "verdict": "unknown", "ts": when,
			}},
		},
		{
			name: "pinned under another name",
			line: Line{Time: when, Target: "alice", Kind: "message", From: "al", Key: other, Verdict: "renamed", As: "alice", Text: "psst"},
			want: &channelParams{Content: "psst", Meta: map[string]string{
				"target": "alice", "from": "al", "key": other, "verdict": "renamed", "as": "alice", "ts": when,
			}},
		},
		{
			name: "unsigned",
			line: Line{Time: when, Target: "#go", Kind: "message", From: "alice", Verdict: "unattributed", Text: "hi"},
			want: &channelParams{Content: "hi", Meta: map[string]string{
				"target": "#go", "from": "alice", "verdict": "unattributed", "ts": when,
			}},
		},
		{
			name: "continuation",
			line: Line{Time: when, Target: "#go", Kind: "message", From: "alice", Key: other, Verdict: "unknown", Wrapped: true, Text: "more"},
			want: &channelParams{Content: "more", Meta: map[string]string{
				"target": "#go", "from": "alice", "key": other, "verdict": "unknown", "wrapped": "true", "ts": when,
			}},
		},
		{
			name: "own line",
			line: Line{Time: when, Target: "#go", Kind: "message", From: "scribe", Key: self, Text: "mine"},
		},
		{
			name: "own nick, another key",
			line: Line{Time: when, Target: "#go", Kind: "message", From: "scribe", Key: other, Verdict: "claimed", Text: "not mine"},
			want: &channelParams{Content: "not mine", Meta: map[string]string{
				"target": "#go", "from": "scribe", "key": other, "verdict": "claimed", "ts": when,
			}},
		},
		{
			name: "event",
			line: Line{Time: when, Target: "#go", Kind: "event", Text: "alice has joined"},
		},
		{
			name: "notice",
			line: Line{Time: when, Target: "#go", Kind: "notice", From: "irciroh", Text: "topic"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := channelEvent(tt.line, self)
			if ok != (tt.want != nil) {
				t.Fatalf("channelEvent ok = %v, want %v", ok, tt.want != nil)
			}
			if ok && !reflect.DeepEqual(got, *tt.want) {
				t.Errorf("channelEvent = %+v, want %+v", got, *tt.want)
			}
		})
	}
}

// A captureConn records what a server would have written to its client.
type captureConn struct {
	mcp.Connection
	mu   sync.Mutex
	sent []*jsonrpc.Request
}

func (c *captureConn) Write(ctx context.Context, msg jsonrpc.Message) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sent = append(c.sent, msg.(*jsonrpc.Request))
	return nil
}

func (c *captureConn) take() []*jsonrpc.Request {
	c.mu.Lock()
	defer c.mu.Unlock()
	sent := c.sent
	c.sent = nil
	return sent
}

// TestChannelSweep is channelEvent meeting a real log: the backlog is
// not pushed, a new line is pushed once, and this view's own is not.
func TestChannelSweep(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	self, other := strings.Repeat("ab", 32), strings.Repeat("cd", 32)
	if err := os.WriteFile(filepath.Join(root, "key"), []byte(self+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	say := func(target, key, from, text string) {
		t.Helper()
		dir := filepath.Join(root, target)
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		line := "@k=" + key + " 2026-11-11T17:00:00Z <" + from + "> " + text
		if err := appendLines(filepath.Join(dir, "out"), []string{line}); err != nil {
			t.Fatal(err)
		}
	}
	say("#go", other, "alice", "said before this view started")

	m := newMCPServer(root, true)
	conn := new(captureConn)
	m.conn = conn

	say("#go", other, "alice", "too early")
	m.sweep(ctx)
	if got := conn.take(); len(got) != 0 {
		t.Fatalf("pushed %d events before the client initialized", len(got))
	}
	m.ready = true

	say("#go", self, "scribe", "my own")
	say("alice", other, "alice", "a private one")
	m.sweep(ctx)
	m.sweep(ctx) // nothing new: nothing more
	got := conn.take()
	var texts []string
	for _, req := range got {
		if req.Method != channelMethod || req.IsCall() {
			t.Errorf("wrote %s (call=%v), want a %s notification", req.Method, req.IsCall(), channelMethod)
		}
		var p channelParams
		if err := json.Unmarshal(req.Params, &p); err != nil {
			t.Fatal(err)
		}
		if p.Meta["key"] != other {
			t.Errorf("event %q carries key %q, want %q", p.Content, p.Meta["key"], other)
		}
		texts = append(texts, p.Meta["target"]+": "+p.Content)
	}
	want := []string{"#go: too early", "alice: a private one"}
	if !reflect.DeepEqual(texts, want) {
		t.Errorf("pushed %q, want %q", texts, want)
	}

	for range channelBurst + 1 {
		say("#go", other, "alice", "flood")
	}
	m.sweep(ctx)
	if got := conn.take(); len(got) != 1 || !strings.Contains(string(got[0].Params), "21 new lines in #go") {
		t.Errorf("a flood pushed %d events, want one saying how many lines there are", len(got))
	}
}

func TestChannelCapability(t *testing.T) {
	for _, on := range []bool{false, true} {
		ctx := context.Background()
		server, client := mcp.NewInMemoryTransports()
		m := newMCPServer(t.TempDir(), on)
		ss, err := m.s.Connect(ctx, server, nil)
		if err != nil {
			t.Fatal(err)
		}
		cs, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil).Connect(ctx, client, nil)
		if err != nil {
			t.Fatal(err)
		}
		res := cs.InitializeResult()
		if _, ok := res.Capabilities.Experimental[channelCapability]; ok != on {
			t.Errorf("channel=%v: capability declared = %v", on, ok)
		}
		if got := strings.Contains(res.Instructions, "<channel source="); got != on {
			t.Errorf("channel=%v: instructions describe channel events = %v", on, got)
		}
		cs.Close()
		ss.Close()
	}
}
