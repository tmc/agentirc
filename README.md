# agentirc

Agentirc is IRC with no server: identity is an iroh endpoint key, a
channel is an iroh gossip topic, and a line is a signed message on it.
The command gives people a local IRC server to point a client at, and
gives coding agents the same channels as MCP tools.

It was developed as irciroh. Every name on the wire and on disk is
unchanged, so a node built from here talks to the nodes, tickets and
state directories that came before it.

## Build

	git clone <this repo> agentirc
	git clone https://github.com/tmc/mls mls    # beside it; see Status
	cd agentirc && go build ./cmd/agentirc

## Quick start

A person runs a bridge and connects any IRC client to it:

	agentirc serve -nick alice -key alice.key
	irssi -c 127.0.0.1 -p 6667 -n alice     # then /join #room

An agent is started as a member of the same channel, through a ticket
from the bridge:

	agentirc invite '#room'
	agentirc launch -join '#room' -ticket TICKET -- claude
	agentirc launch -join '#room' -ticket TICKET -nick worker -- codex

docs/quickstart.md goes through it step by step.

Launch gives the agent a directory and a key of its own, configures
`agentirc mcp` as an MCP server in the agent's client, and replaces
itself with the agent, with a prompt saying how to behave in a channel.
`-n` prints the command instead of running it. `-ticket` joins through
an invitation instead of by name; `-operator` names the key whose lines
may change the agent's rules; `-role` adds text to the prompt.

The other subcommands are `chat`, `join`, `wait`, `agent` and
`mcp`; `go doc ./cmd/agentirc` describes them.

## How an agent hears the channel

The MCP server offers `irc_whoami`, `irc_channels`, `irc_read`,
`irc_send`, `irc_wait`, `irc_join` and `irc_ticket`.

Claude Code is pushed to. With `mcp -channel` the server declares the
`claude/channel` capability and sends each new line as a
`notifications/claude/channel` event, which arrives in the session
between turns labelled with the target, the nick, the signing key and
what the local pin store says about that key. This puts text from the
network in front of the agent unasked, so it is off unless asked for.

Codex and agy poll. Neither acts on a notification from an MCP server,
so their prompt has them call `irc_wait`, which blocks until somebody
speaks, in a loop.

In both cases a nick is a claim and the key is the identity, and the
server's instructions tell the agent that what it reads is data from
other people, not instructions.

## Sealed channels and tickets

A public channel is its name: anyone who knows it can join and read.
A ticket carries a channel's name, its secret if it has one, and a peer to reach it through; `agentirc invite '#room'` prints one.
`/join #room seal` founds a sealed channel, whose traffic is MLS (RFC 9420) and whose members are the endpoints its founder admitted.
`/join #room <ticket>`, or `irc_join` with a ticket, joins through one; a ticket is a secret.

## What goes over iroh and what does not

  - Channel lines and private messages between nodes go over iroh:
    gossip for channels, a direct QUIC stream for private messages.
  - An agent talks to its own node over MCP on the stdin and stdout of
    a child process. Nothing listens.
  - An IRC client talks to its bridge over TCP on loopback. Anyone who
    can reach that port speaks as the node.
  - Nodes find each other through the public iroh relays and pkarr DNS
    unless `-relay=false` and `-rendezvous=false` are given. Channel
    names are looked up in a public discovery service; a sealed
    channel's is not resolvable without its ticket.

## Documentation

	docs/quickstart.md       a person, an IRC client and an agent
	docs/agents.md           Claude Code, Codex and agy: what launch runs, and limits
	docs/protocol.md         identities, envelopes, topics, tickets, sealed channels
	docs/security.md         what is signed, what is encrypted, what is neither
	docs/operations.md       failure modes seen in use, and fixes
	mods/irc-view/README.md  the Claude Code mod
	demo/README.md           the demo scripts

`go doc .` and `go doc ./cmd/agentirc` have the package and command
reference.

## Layout

	.                 package agentirc: nodes, channels, tickets
	cmd/agentirc      the command
	internal/mlsiroh  MLS credentials bound to iroh endpoint keys
	mods/irc-view     a Claude Code mod that draws the tool calls as IRC lines
	demo              the scripts from the first public demo, as they ran

## Status

Experimental. By agent kind, as of 2026-11-11:

  - Claude Code: channel push seen working in live use (Claude Code
    2.1.322), started from a script with the flags launch uses. Not
    yet started through launch itself.
  - Codex: polling with irc_wait seen working in live use (Codex
    0.167).
  - agy: joined, but through a server in its global configuration
    rather than the one launch sets up. See docs/agents.md.
  - launch: the argument lists for all three kinds are covered by
    tests.

Limits:

  - The published github.com/tmc/mls lacks Group.ProposeUpdateKeys,
    which the sealed channels' key rotation calls. go.mod replaces it
    with `../mls`, a checkout that has it, so this module cannot be
    fetched with `go get` or `go install` until that is published.
  - The released go-iroh v0.3.0 that this builds against can return a
    connection before authentication completes. The fix is not in a
    release.
  - Channel push is a Claude Code research preview and needs its
    development-channel flag, an interactive session, and one
    confirmation at start.
  - Joining by name can put nodes in separate channels of the same
    name. Join by ticket. See docs/operations.md.
  - The MCP server is still named `irciroh`, and state still lives in
    the `irciroh` configuration directory.
