package agentirc

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/tmc/go-iroh/dns"
	"github.com/tmc/go-iroh/gossip"
	"github.com/tmc/go-iroh/iroh"
	"github.com/tmc/go-iroh/netaddr"
	"github.com/tmc/go-iroh/relay"
)

// TestQualificationPeer is a diagnostic child, selected only by its environment.
// It uses real endpoints and protocol handlers. Its JSON controls expose test
// hooks, not a production interface. No private key or MLS state is returned.
func TestQualificationPeer(t *testing.T) {
	if os.Getenv("IRCIROH_QUALIFICATION_CHILD") != "1" {
		return
	}
	// TestMain speeds up ordinary tests. These children retain production
	// presence and rotation intervals, including the natural-timer observation.
	presenceInterval, presenceTimeout, presenceSweep = 30*time.Second, 90*time.Second, 10*time.Second
	rotateInterval = 15 * time.Minute
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 40*time.Minute)
	defer cancel()
	p, err := newQualificationPeer(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer p.close()
	out := json.NewEncoder(os.Stdout)
	if err := out.Encode(map[string]any{"ready": p.node.ID().String(), "force_relay": p.forceRelay,
		"controlled_finder": p.controlled, "presence_seconds": presenceInterval.Seconds(),
		"rotation_seconds": rotateInterval.Seconds()}); err != nil {
		t.Fatal(err)
	}
	commands := make(chan qualificationRequest)
	go func() {
		defer close(commands)
		s := bufio.NewScanner(os.Stdin)
		s.Buffer(make([]byte, 4096), 64*1024)
		for s.Scan() {
			var r qualificationRequest
			if json.Unmarshal(s.Bytes(), &r) != nil {
				continue
			}
			select {
			case commands <- r:
			case <-ctx.Done():
				return
			}
		}
	}()
	for {
		select {
		case <-ctx.Done():
			return
		case r, ok := <-commands:
			if !ok || r.Op == "stop" {
				return
			}
			value, err := p.command(ctx, r)
			if err != nil {
				_ = out.Encode(map[string]string{"error": err.Error()})
			} else {
				_ = out.Encode(map[string]any{"result": value})
			}
			if r.Op == "crash" {
				// Simulate abrupt loss of only this dedicated child.
				os.Exit(0)
			}
		}
	}
}

type qualificationRequest struct {
	Op      string `json:"op"`
	Channel string `json:"channel"`
	Ticket  string `json:"ticket"`
	Text    string `json:"text"`
	Match   string `json:"match"`
	Seconds int    `json:"seconds"`
	Sender  string `json:"sender"`
}

type qualificationPeer struct {
	node       *Node
	router     *iroh.Router
	publisher  *iroh.PkarrPublisher
	forceRelay bool
	controlled bool
	mu         sync.Mutex
	hint       netaddr.EndpointAddr
	haveHint   bool
	lookups    []string
	messages   map[string][]Event
	recording  map[*Channel]bool
}

func newQualificationPeer(ctx context.Context) (*qualificationPeer, error) {
	p := &qualificationPeer{forceRelay: os.Getenv("IRCIROH_FORCE_RELAY") == "1",
		controlled: os.Getenv("IRCIROH_CONTROLLED_FINDER") == "1", messages: make(map[string][]Event), recording: make(map[*Channel]bool)}
	directory := os.Getenv("IRCIROH_PEER_DIR")
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	root := filepath.Join(home, "tmp")
	rel, err := filepath.Rel(root, directory)
	if directory == "" || !filepath.IsAbs(directory) || err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil, errors.New("peer directory must be an absolute owned directory under ~/tmp")
	}
	resolved, err := filepath.EvalSymlinks(directory)
	if err != nil || resolved != filepath.Clean(directory) {
		return nil, errors.New("peer directory must exist and contain no symlinks")
	}
	lookup := &iroh.AddressLookupServices{}
	resolver, err := iroh.NewPkarrResolver(iroh.N0DNSPkarrRelayProd, nil)
	if err != nil {
		return nil, err
	}
	lookup.AddResolver(resolver)
	opts := []iroh.Option{iroh.WithRelayMode(relay.ModeDefault()), iroh.WithAddressLookup(lookup)}
	if p.forceRelay {
		opts = append(opts, iroh.WithoutIPTransports())
	}
	ep, err := iroh.Bind(ctx, opts...)
	if err != nil {
		return nil, err
	}
	defer func() {
		if p.node == nil {
			cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_ = ep.Shutdown(cleanup)
		}
	}()
	online, cancel := context.WithTimeout(ctx, 60*time.Second)
	err = ep.Online(online)
	cancel()
	if err != nil {
		return nil, fmt.Errorf("relay online: %w", err)
	}
	p.publisher, err = iroh.N0PkarrPublisher(ep.SecretKey(), nil)
	if err != nil {
		return nil, err
	}
	g := gossip.NewGossip(ep)
	p.node, err = NewNode(ep, g, os.Getenv("IRCIROH_PEER_NAME"),
		WithStateDir(filepath.Join(directory, "state")), WithRendezvous())
	if err != nil {
		p.publisher.Close()
		g.Shutdown(ctx)
		return nil, err
	}
	find := p.node.find
	if p.controlled {
		// Controlled records and an accelerated periodic merge are explicit
		// fixtures. No package-wide timing variable or production API changes.
		p.node.rv = nil
		p.node.watch, p.node.merge = 200*time.Millisecond, 2*time.Second
	}
	p.node.find = func(ctx context.Context, channel string, secret []byte) (netaddr.EndpointAddr, error) {
		var addr netaddr.EndpointAddr
		var err error
		if p.controlled {
			p.mu.Lock()
			addr, err = p.hint, ErrNoRendezvous
			if p.haveHint {
				err = nil
			}
			p.mu.Unlock()
		} else {
			addr, err = find(ctx, channel, secret)
		}
		p.mu.Lock()
		if err == nil {
			p.lookups = append(p.lookups, addr.ID.String())
		} else {
			p.lookups = append(p.lookups, "error: "+err.Error())
		}
		p.mu.Unlock()
		return addr, err
	}
	p.router, err = iroh.NewRouter(ep, map[string]iroh.ProtocolHandler{
		gossip.ALPN: g.Handler(), ALPN: p.node.Handler(), InviteALPN: p.node.InviteHandler(),
		HistoryALPN: p.node.HistoryHandler(),
	}, nil)
	if err != nil {
		p.close()
		return nil, err
	}
	p.publisher.Publish(dns.NewEndpointData(p.node.Addr().Addrs()...))
	return p, nil
}

func (p *qualificationPeer) close() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if p.node != nil {
		_ = p.node.Close()
	}
	if p.router != nil {
		_ = p.router.Shutdown(ctx)
	}
	if p.node != nil {
		p.node.g.Shutdown(ctx)
		_ = p.node.ep.Shutdown(ctx)
	}
	if p.publisher != nil {
		p.publisher.Close()
	}
}

func (p *qualificationPeer) record(c *Channel) {
	p.mu.Lock()
	if p.recording[c] {
		p.mu.Unlock()
		return
	}
	p.recording[c] = true
	p.mu.Unlock()
	go func() {
		for event, err := range c.Events() {
			if err == nil && event.Kind == Message {
				p.mu.Lock()
				p.messages[c.Name()] = append(p.messages[c.Name()], event)
				p.mu.Unlock()
			}
		}
	}()
}

func (p *qualificationPeer) command(parent context.Context, r qualificationRequest) (any, error) {
	seconds := r.Seconds
	if seconds <= 0 || seconds > 120 {
		seconds = 60
	}
	ctx, cancel := context.WithTimeout(parent, time.Duration(seconds)*time.Second)
	defer cancel()
	switch r.Op {
	case "crash":
		return "owned child crash", nil
	case "found":
		c, ticket, err := p.node.Found(ctx, r.Channel, nil)
		if err == nil {
			p.record(c)
		}
		return ticket.String(), err
	case "join":
		ticket := Ticket{Channel: r.Channel}
		if r.Ticket != "" {
			var err error
			ticket, err = ParseTicket(r.Ticket)
			if err != nil {
				return nil, err
			}
		}
		c, err := p.node.JoinTicket(ctx, ticket)
		if err == nil {
			p.record(c)
		}
		return "joined", err
	case "ticket":
		t, err := p.node.Ticket(r.Channel)
		return t.String(), err
	case "set_hint":
		if !p.controlled {
			return nil, errors.New("controlled finder disabled")
		}
		t, err := ParseTicket(r.Ticket)
		if err != nil || len(t.Peers) != 1 {
			return nil, errors.New("hint must contain exactly one peer")
		}
		p.mu.Lock()
		p.hint, p.haveHint = t.Peers[0], true
		p.mu.Unlock()
		return t.Peers[0].ID.String(), nil
	case "lookup":
		addr, err := p.node.find(ctx, r.Channel, nil)
		return map[string]any{"id": addr.ID.String(), "addresses": addr.Addrs()}, err
	case "status":
		return p.status(r.Channel), nil
	case "wait":
		if r.Sender == "" || r.Match == "" {
			return nil, errors.New("wait requires sender identity and unique marker")
		}
		for {
			p.mu.Lock()
			for _, event := range p.messages[r.Channel] {
				if event.Text == r.Match && event.From.ID.String() == r.Sender {
					p.mu.Unlock()
					return event.Text, nil
				}
			}
			p.mu.Unlock()
			select {
			case <-ctx.Done():
				return nil, fmt.Errorf("waiting for marker: %w", ctx.Err())
			case <-time.After(100 * time.Millisecond):
			}
		}
	}
	c, ok := p.node.Channel(r.Channel)
	if !ok {
		return nil, ErrNoChannel
	}
	switch r.Op {
	case "say":
		return "sent", c.Say(ctx, r.Text)
	case "part":
		return "parted", c.Part(ctx, "diagnostic fixture")
	case "rotate":
		if c.seal == nil {
			return nil, ErrNotSealed
		}
		return "proposal sent", c.seal.rotate(ctx)
	case "flush":
		if c.seal == nil {
			return nil, ErrNotSealed
		}
		return "staged updates committed", c.seal.flush(ctx)
	}
	return nil, errors.New("unknown diagnostic operation")
}

func (p *qualificationPeer) status(channel string) map[string]any {
	result := map[string]any{"id": p.node.ID().String(), "timestamp": time.Now().UTC()}
	p.mu.Lock()
	result["lookups"] = append([]string{}, p.lookups...)
	p.mu.Unlock()
	c, ok := p.node.Channel(channel)
	if !ok {
		return result
	}
	neighbors, members := []string{}, []string{}
	paths := map[string]any{}
	for _, id := range c.Neighbors() {
		neighbors = append(neighbors, id.String())
		if info, ok := p.node.ep.RemoteInfo(id); ok {
			var addresses []map[string]any
			for _, addr := range info.Addrs {
				addresses = append(addresses, map[string]any{"address": addr.Addr.String(),
					"active": addr.Usage == iroh.TransportAddrActive, "provenance": addr.Provenance})
			}
			paths[id.String()] = addresses
		}
	}
	for _, member := range c.Members() {
		members = append(members, member.ID.String())
	}
	result["neighbors"], result["members"], result["paths"] = neighbors, members, paths
	if c.seal != nil {
		c.seal.mu.Lock()
		defer c.seal.mu.Unlock()
		if c.seal.group == nil {
			return result
		}
		result["epoch"], result["staged"] = c.seal.group.Epoch(), len(c.seal.pending)
		result["last_resync"] = c.seal.lastResync
		if leaf := c.seal.group.Tree.Leaf(c.seal.group.Index); leaf != nil {
			enc, sig := sha256.Sum256(leaf.EncryptionKey), sha256.Sum256(leaf.SignatureKey)
			result["encryption_key_sha256"] = hex.EncodeToString(enc[:])
			result["signing_key_sha256"] = hex.EncodeToString(sig[:])
		}
	}
	return result
}
