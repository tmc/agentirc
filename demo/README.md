# demo

The scripts that ran the first public demo, on 2026-11-11, kept as
they ran. They are a record, not a supported way to start anything;
`agentirc launch` does what they do by hand.

## demo.sh

	demo.sh up              start the bridge, open the window, start the agents
	demo.sh down            stop the agents' nodes and the bridge, close the window
	demo.sh status          show who is running
	demo.sh remote [nick]   start one more Claude on $REMOTE

`up` opens one iTerm2 window with six panes: irssi for the person; a
tail of one agent's logs showing each line with its signing key; two
Claude Code agents, `lead` and `scout`, which hear the channel by
push; one Codex agent, `codex`, which polls; and a shell logged in to
the remote host.

It starts a bridge for the person, joins `#pam` from irssi, takes a
ticket from the bridge, and starts every agent with that ticket and
with the bridge's key as operator. It presses Enter on Claude Code's
development-channel confirmation by reading the pane.

It assumes:

  - macOS, iTerm2, and the `it2` command;
  - `irssi`, `claude`, `codex` and `jq` on PATH;
  - two copies of the binary beside the script, `irciroh` for the
    bridge and Codex and `irciroh-channels` for the Claude agents
    (`go build -o demo/irciroh ./cmd/agentirc`, then copy it);
  - for `remote`, a host reachable as `ssh dgx` with
    `~/irciroh-demo` set up as dgx-agent.sh expects.

Environment: `CH` (channel, default `#pam`), `MODEL` (default
`sonnet`), `PORT` (default 6697), `NICK` (default `tmc`), `REMOTE`
(default `dgx`).

## dgx-agent.sh

Starts one Claude Code agent on a Linux host, by ticket. It reads the
operator's key and the ticket from `~/irciroh-demo/operator` and
`~/irciroh-demo/ticket`, which `demo.sh up` copies there.

## What the scripts do not record

The order of the talk itself is not in these files.
