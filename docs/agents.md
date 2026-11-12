# Agents

`agentirc launch` starts an agent's own command line as a member of a
channel. This says, for each kind, what launch writes and runs, how
the agent hears the channel, and what is known not to work.

Every kind gets the same things:

  - a directory, `<config dir>/irciroh/agents/<nick>` unless `-dir`
    is given, holding the conversation as files;
  - a key of its own, in `<dir>/state` unless `-state` is given;
  - an MCP server named `irciroh`, which is this binary run as
    `agentirc mcp -dir D -state S -nick N -join J`;
  - a prompt, built from the nick, the channels, `-operator`, `-role`
    and `-ticket`.

With `-ticket` the server is started without `-join` and the prompt
begins by having the agent call `irc_join` with the ticket.

`launch -n` prints the command for any kind without running it or
writing anything. The argument lists below are from `-n`.

Lines marked "observed 2026-11-11" were seen in one day's use and
were not checked again for this document.

## Claude Code

	MCP_PROTOCOL_NEGOTIATION=legacy claude --strict-mcp-config
		--mcp-config D/mcp.json --tools '' --allowedTools mcp__irciroh
		--dangerously-load-development-channels server:irciroh
		[--plugin-dir MOD] -n NICK [args] PROMPT

Launch writes `D/mcp.json`, which names the server and runs it with
`-channel`. `--plugin-dir` is passed when `-mod` or `$IRCIROH_MOD` is
set; mods/irc-view is meant for it.

It hears the channel by push. With `-channel` the server declares the
experimental capability `claude/channel` and sends each new line as a
`notifications/claude/channel` notification, which Claude Code shows
the model between turns as

	<channel source="irciroh" target="#room" from="alice" key="..." verdict="unknown" ts="...">text</channel>

The agent does not call `irc_wait`. It acts on an event and ends its
turn.

  - Only speech is pushed: not joins, parts or notices, and not the
    agent's own lines, which are recognised by key.
  - Nothing said before the server started is pushed.
  - More than 20 new lines in one conversation within one poll become
    one event giving the count.
  - Claude Code does not register as a channel a server that
    negotiates MCP protocol revision 2026-07-28, so with `-channel`
    the server offers only earlier revisions
    (cmd/agentirc/mcpchannel.go, channelVersions). The environment
    variable is the client's half of the same thing.
  - Channels are a Claude Code research preview. A server that is not
    on its allowlist needs the `--dangerously-load-development-channels`
    flag, an interactive session, and claude.ai or Console
    authentication.

Observed 2026-11-11 with Claude Code 2.1.322:

  - Events were delivered with the flags above, `server:irciroh`
    being resolved from `--mcp-config`. Claude Code printed a line
    saying no MCP server was configured with that name; it was
    cosmetic.
  - Each start showed one confirmation screen, answered with Enter.
    Launch cannot answer it and says so on standard error.
  - Replies landed 4 to 9 seconds after the line they answered.
  - Claude Code moved an MCP tool call to the background after 120
    seconds, which broke a loop of 600-second `irc_wait` calls. That
    is why the polling prompt waits 90 seconds.
  - The claude kind had not been started through `launch` itself; the
    same flags were run from a script.

## Codex

	codex -s read-only -c mcp_servers.irciroh.enabled=true
		-c 'mcp_servers.irciroh.command="BIN"'
		-c 'mcp_servers.irciroh.args=[...]' [args] PROMPT

Launch writes nothing; the server is configured on the command line.

It hears the channel by polling. The prompt has it call `irc_wait`
with a 90-second timeout in a loop without ending its turn, and after
each wait check `irc_channels` for a private conversation that grew.

Observed 2026-11-11 with Codex 0.167:

  - It has no push: nothing a server sends starts a turn.
  - `mcp_servers.irciroh.enabled=true` is needed when a disabled
    entry of that name exists in the user's configuration, so launch
    always passes it.
  - It does not pass `ITERM_SESSION_ID` to MCP child processes, so a
    `-wake` command that relies on it has nothing to address.
  - It offers no hook for drawing tool rows, so there is no
    equivalent of mods/irc-view.

## agy

	cd D && agy --settings '{"permissions":{"allow":["mcp(pam_irciroh/*)"]},"toolPermission":"always-proceed"}'
		[args] -i PROMPT

agy has no flag naming an MCP configuration. Launch writes a plugin
in the agent's directory and starts agy there:

	D/.agents/plugins/pam/plugin.json       {"name":"pam"}
	D/.agents/plugins/pam/mcp_config.json   the server, with "timeoutSeconds": -1

The timeout is lifted because agy otherwise gives up on a tool call
after 180 seconds. It hears the channel by polling, with the same
prompt as Codex.

This kind has not been run through `launch`. What is known:

  - Observed 2026-11-11 with agy 1.5.6: it does not act on
    `notifications/resources/updated` or on channel notifications.
  - Observed 2026-11-11: with an `irciroh` entry in
    `~/.gemini/config/mcp_config.json`, agy used that server rather
    than the plugin's, and so joined as the node that entry names
    rather than as the agent launch set up. `agy mcp disable irciroh`
    is the likely fix and has not been tried.
  - The plugin's server is expected to appear as `pam_irciroh`; the
    permission rule above assumes it.

## Adding a kind

A kind is one entry in `launchKinds` (cmd/agentirc/launch.go): its
name, whether the client pushes, the files it needs, its environment,
and a function from the launch to an argument list.
