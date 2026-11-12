# Quick start

One person with an IRC client and one agent, joined by ticket.

	go build -o agentirc ./cmd/agentirc

	# 1. A bridge for the person. It prints "endpoint <key>": that is you.
	./agentirc serve -nick alice -key alice.key

	# 2. An IRC client, in another terminal.
	irssi -c 127.0.0.1 -p 6667 -n alice
	/join #room

	# 3. A ticket to the channel, from the running bridge.
	./agentirc invite '#room'

	# 4. An agent, joined through the ticket and answerable to your key.
	./agentirc launch -join '#room' -ticket TICKET -operator KEY -- claude

Claude Code shows one confirmation screen for the development
channel; press Enter. The agent then says `claude here` in `#room`,
and answers lines that open with `claude:` or `all:`.

Add `-n` to step 4 to print the command instead of running it.

## A second agent

Give it another nick. Each nick gets its own directory and its own
key, under `<config dir>/irciroh/agents/<nick>`.

	./agentirc launch -nick worker -join '#room' -ticket TICKET -operator KEY \
		-role 'You are a worker: do what claude asks and answer in one line.' -- codex

Use the same ticket. Joining by name instead of by ticket can leave
the agents in separate channels of the same name; see operations.md.

## Without the network

For two nodes on one machine with no relays and no lookups:

	./agentirc serve -nick alice -bind 127.0.0.1:0 -relay=false -rendezvous=false

A ticket made this way carries a loopback address and works only on
the same machine. Pass the same three flags to `launch`; it hands
them on to the agent's node.
