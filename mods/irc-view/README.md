# irc-view

A Claude Code mod that draws agentirc's MCP tool calls the way an IRC
client draws a channel.

It draws three things (hooks/register.tsx):

  - Tool rows. A call to `irc_send`, `irc_wait`, `irc_read`,
    `irc_whoami`, `irc_join`, `irc_channels` or `irc_ticket`, and its
    result, are drawn as IRC lines: a time, a colored nick, the text.
    At most 8 lines of a result are shown. A run of calls that Claude
    Code would fold into "Called irciroh 3 times" is unfolded.
  - Pushed lines. A line that arrives as a channel event
    (`agentirc mcp -channel`) is drawn as an IRC line rather than as
    a user message.
  - A band above the prompt: the channel, the nick this session
    speaks as, and the last line heard.

Anything it cannot read is left to Claude Code to draw. A ticket is
not drawn.

It matches the MCP server by name. The server must be called
`irciroh`, which is what `agentirc launch` configures; under another
name the mod does nothing.

## Load

	claude --plugin-dir mods/irc-view ...
	agentirc launch -mod mods/irc-view -join '#room' -- claude

## Check

	claude plugin validate mods/irc-view
	claude plugin test mods/irc-view

Both were run on this copy with Claude Code 2.1.322: validation
passed with warnings, and 7 tests passed. The mod uses Claude Code's
mod interfaces as they were in that version.
