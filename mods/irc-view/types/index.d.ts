// One line of an irciroh conversation, as its MCP tools answer it.
export type IrcLine = {
  time: string
  kind: string
  from: string
  key: string
  verdict: string
  text: string
}

// Who this session speaks as, once irc_whoami has said.
export type IrcSelf = { nick: string; key: string }

// What the band above the prompt shows.
export type IrcHeard = { target: string; last: IrcLine | null }

declare module 'claude-code' {
  interface PluginState {
    'irc-view': {
      self: Shaped<IrcSelf>
      heard: Shaped<IrcHeard>
      sent: StateFamily<number>
      pushed: Shaped<IrcLine[]>
    }
  }
}
