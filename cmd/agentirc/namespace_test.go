package main

import (
	"testing"

	"github.com/tmc/go-iroh-experiments/irciroh"
)

func TestNamespaceSecret(t *testing.T) {
	topic := func(ns, ch string) [32]byte {
		var secret []byte
		if ns != "" {
			secret = namespaceSecret(ns, ch)
		}
		return [32]byte(irciroh.TopicID(ch, secret))
	}
	for _, tt := range []struct {
		name     string
		ns1, ch1 string
		ns2, ch2 string
		same     bool
	}{
		{"same namespace and channel", "k", "#agents", "k", "#agents", true},
		{"channel names fold", "k", "#Agents", "k", "#agents", true},
		{"another namespace", "k", "#agents", "j", "#agents", false},
		{"no namespace", "k", "#agents", "", "#agents", false},
		{"another channel", "k", "#agents", "k", "#ops", false},
	} {
		if got := topic(tt.ns1, tt.ch1) == topic(tt.ns2, tt.ch2); got != tt.same {
			t.Errorf("%s: same topic = %v, want %v", tt.name, got, tt.same)
		}
	}
}
