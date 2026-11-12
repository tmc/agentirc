// irc-view draws irciroh's MCP tool calls the way an IRC client draws a
// channel: one line per thing said, a time, a colored nick, the text.
//
// A tool row is two render sites. ToolUse is the call: what was sent, or
// what is being waited for. ToolResult is the answer: the lines heard.
// Anything this module cannot read is left to the engine with next(e).
import { atom, memberOf, read, update } from 'claude-code'
import type { Elements, Register, RenderElement, StateDollar } from 'claude-code'

import type { IrcHeard, IrcLine, IrcSelf } from '../types'

const TOOL = /^mcp__irciroh__irc_(send|wait|read|whoami|join|channels|ticket)$/
const MAX_LINES = 8
const PALETTE = [
  '#61afef', '#98c379', '#e5c07b', '#c678dd',
  '#56b6c2', '#e06c75', '#d19a66', '#ff79c6',
]

const self = atom({ plugin: 'irc-view', key: 'self' } as const, { nick: '', key: '' }, { shape: 'self-1' })
const heard = atom({ plugin: 'irc-view', key: 'heard' } as const, { target: '', last: null }, { shape: 'heard-1' })
const sent = atom({ plugin: 'irc-view', key: 'sent' } as const, 0)
// The lines pushed as channel events, newest last, for their rows to find.
const pushed = atom({ plugin: 'irc-view', key: 'pushed' } as const, [], { shape: 'pushed-1' })
const MAX_PUSHED = 50

type Record_ = { readonly [key: string]: unknown }
type Table = Pick<Elements['terminal'], 'Box' | 'Text'>

const isRecord = (v: unknown): v is Record_ =>
  typeof v === 'object' && v !== null && !Array.isArray(v)

const str = (v: unknown): string => (typeof v === 'string' ? v : '')

// verb names which irciroh tool a full tool name is, or '' for any other.
const verb = (tool: string): string => TOOL.exec(tool)?.[1] ?? ''

// record finds a tool's structured result. It may arrive as the object
// itself, inside an MCP result envelope, or as JSON in a text block.
function record(v: unknown, depth = 0): Record_ | null {
  if (depth > 4) return null
  if (typeof v === 'string') {
    try {
      return record(JSON.parse(v), depth + 1)
    } catch {
      return null
    }
  }
  if (Array.isArray(v)) {
    for (const block of v) {
      const found = record(isRecord(block) && 'text' in block ? block.text : block, depth + 1)
      if (found !== null) return found
    }
    return null
  }
  if (!isRecord(v)) return null
  if (isRecord(v.structuredContent)) return v.structuredContent
  if (Array.isArray(v.content)) return record(v.content, depth + 1)
  return v
}

// logText finds the log as text, the other half of an irciroh result,
// for when the structured half did not come through.
function logText(v: unknown): string {
  if (typeof v === 'string') return v
  if (Array.isArray(v)) return v.map(logText).join('\n')
  if (isRecord(v)) return logText(v.text ?? v.content ?? '')
  return ''
}

const LOG = /^(\d{4}-\d\d-\d\dT\S+) (?:<([^>[\s]+)(?:\[(\w+)\])?[^>]*> (.*)|-!- (.*)|-([^-[\s]+)(?:\[(\w+)\])?[^-]*- (.*))$/

function parseLog(text: string): IrcLine[] {
  const lines: IrcLine[] = []
  for (const row of text.split('\n')) {
    const m = LOG.exec(row)
    if (m === null) continue
    const [, time = '', from, key, said, event, noticeFrom, noticeKey, notice] = m
    if (said !== undefined) {
      lines.push({ time, kind: 'message', from: from ?? '', key: key ?? '', verdict: '', text: said })
    } else if (event !== undefined) {
      lines.push({ time, kind: 'event', from: '', key: '', verdict: '', text: event })
    } else {
      lines.push({ time, kind: 'notice', from: noticeFrom ?? '', key: noticeKey ?? '', verdict: '', text: notice ?? '' })
    }
  }
  return lines
}

// linesOf answers the lines an irc_wait or irc_read result holds, or null
// when the result is not one this module can read.
function linesOf(output: unknown): IrcLine[] | null {
  const out = record(output)
  if (out !== null && 'lines' in out) {
    if (out.lines === null) return []
    if (!Array.isArray(out.lines)) return null
    return out.lines.filter(isRecord).map(l => ({
      time: str(l.time),
      kind: str(l.kind),
      from: str(l.from),
      key: str(l.key),
      verdict: str(l.verdict),
      text: str(l.text),
    }))
  }
  const lines = parseLog(logText(output))
  return lines.length > 0 ? lines : null
}

// hhmm formats an RFC 3339 time, or a clock reading, as the local 15:04.
function hhmm(t: string | number): string {
  const d = new Date(t)
  if (t === '' || t === 0 || Number.isNaN(d.getTime())) return '     '
  const two = (n: number) => String(n).padStart(2, '0')
  return `${two(d.getHours())}:${two(d.getMinutes())}`
}

// colorOf gives a speaker a stable color: by key when there is one, since
// two speakers may claim one nick, else by nick.
function colorOf(nick: string, key: string): string {
  let sum = 0
  for (const c of key || nick) sum = (sum * 31 + c.charCodeAt(0)) >>> 0
  return PALETTE[sum % PALETTE.length] ?? 'text'
}

// row draws one line: `15:58 <scout·a8fdd4> text`, the text hanging
// beside its prefix when it wraps.
function row({ Box, Text }: Table, l: IrcLine): RenderElement {
  const time = <Text dimColor>{hhmm(l.time)} </Text>
  if (l.kind === 'event' || l.kind === 'line') {
    return (
      <Box>
        {time}
        <Text dimColor>{l.kind === 'event' ? '-!- ' : ''}{l.text}</Text>
      </Box>
    )
  }
  const [open, close] = l.kind === 'notice' ? ['-', '-'] : ['<', '>']
  return (
    <Box>
      <Box flexShrink={0}>
        {time}
        <Text>
          {open}
          <Text color={colorOf(l.from, l.key)} weight="bold">{l.from || '?'}</Text>
          {l.key !== '' && <Text dimColor>·{l.key.slice(0, 6)}</Text>}
          {l.verdict === 'claimed' && <Text color="error"> not {l.from}</Text>}
          {close}{' '}
        </Text>
      </Box>
      <Text>{l.text}</Text>
    </Box>
  )
}

// note draws a client's own remark: `      -!- text`, dim.
function note({ Box, Text }: Table, text: string, time: string | number = ''): RenderElement {
  return (
    <Box>
      <Text dimColor>{hhmm(time)} -!- {text}</Text>
    </Box>
  )
}

function column(t: Table, rows: RenderElement[]): RenderElement {
  return <t.Box flexDirection="column">{rows}</t.Box>
}

// log draws the newest lines of a result under a count of the ones left out.
function log(t: Table, lines: IrcLine[]): RenderElement[] {
  const rows = lines.slice(-MAX_LINES).map(l => row(t, l))
  const earlier = lines.length - rows.length
  return earlier > 0 ? [note(t, `+${earlier} earlier`), ...rows] : rows
}

// call draws the ToolUse row from the call's input, or answers null.
function call(t: Table, tool: string, input: unknown, isRunning: boolean, me: IrcSelf, at: number): RenderElement | null {
  if (!isRecord(input)) return null
  const target = str(input.target)
  switch (verb(tool)) {
    case 'send': {
      const said = str(input.text).split('\n').filter(s => s.trim() !== '')
      if (target === '' || said.length === 0) return null
      const mine = (text: string): IrcLine => ({
        time: '', kind: 'message', from: me.nick || 'me', key: me.key, verdict: '', text,
      })
      return column(t, said.map(text => {
        const l = mine(text)
        return row(t, at > 0 ? { ...l, time: new Date(at).toISOString() } : l)
      }))
    }
    case 'wait': {
      if (target === '') return null
      const match = str(input.match)
      const what = match === '' ? '' : ` for /${match}/`
      return note(t, isRunning ? `… listening on ${target}${what}` : `heard on ${target}${what}`)
    }
    case 'read':
      return target === '' ? null : note(t, `reading ${target}`)
    case 'join':
      return target === '' ? null : note(t, `${isRunning ? 'joining' : 'joined'} ${target}`)
    case 'ticket':
      return target === '' ? null : note(t, `ticket for ${target}`)
    case 'whoami':
      return note(t, 'whoami')
    case 'channels':
      return note(t, 'channels')
  }
  return null
}

// answer draws the ToolResult row from the call's output, or answers null.
function answer(t: Table, tool: string, output: unknown): RenderElement | null {
  const out = record(output)
  switch (verb(tool)) {
    case 'wait':
    case 'read': {
      const lines = linesOf(output)
      if (lines === null) return null
      return column(t, lines.length === 0 ? [note(t, 'nothing said')] : log(t, lines))
    }
    case 'send':
      if (out === null || typeof out.sent !== 'number') return null
      // An empty Box: the line is already drawn by the call's own row.
      return out.queued === true ? note(t, 'queued: no chat is attached') : <t.Box />
    case 'whoami':
      if (out === null || str(out.nick) === '') return null
      return note(t, `you are ${str(out.nick)}·${str(out.key).slice(0, 6)}${out.chatting === true ? '' : ' (no chat attached)'}`)
    case 'join':
      return out === null || str(out.target) === '' ? null : note(t, `now talking in ${str(out.target)}`)
    case 'ticket':
      // The ticket is a secret; the model has it, the screen does not.
      return out === null || str(out.ticket) === '' ? null : note(t, `ticket issued for ${str(out.target)} (hidden)`)
    case 'channels': {
      if (out === null || !Array.isArray(out.channels)) return null
      const rows = out.channels.filter(isRecord).map(c =>
        note(t, `${str(c.target)}  ${String(c.lines ?? 0)} lines`, str(c.last)),
      )
      return column(t, rows.length === 0 ? [note(t, 'no conversations')] : rows)
    }
  }
  return null
}

// A channel event as the conversation stores it:
// <channel source="irciroh" target="#agents" from="scout" key="..." ts="...">text</channel>
const CHANNEL = /<channel\b([^>]*)>([\s\S]*?)<\/channel>/g
const ATTR = /([\w-]+)="([^"]*)"/g

// eventsOf answers the irciroh lines a stored row's text holds, each with
// the conversation it was said in.
function eventsOf(text: string, now: number): { target: string; line: IrcLine }[] {
  const events: { target: string; line: IrcLine }[] = []
  for (const [, attrs = '', body = ''] of text.matchAll(CHANNEL)) {
    const a: { [name: string]: string } = {}
    for (const [, name = '', value = ''] of attrs.matchAll(ATTR)) a[name] = value
    if (a.source !== undefined && a.source !== 'irciroh') continue
    const ts = a.ts ?? ''
    // ts is RFC 3339, or a count of seconds or milliseconds.
    const at = /^\d+$/.test(ts) ? Number(ts) * (ts.length > 11 ? 1 : 1000) : Date.parse(ts)
    events.push({
      target: a.target ?? '',
      line: {
        time: new Date(Number.isNaN(at) ? now : at).toISOString(),
        kind: 'message',
        from: a.from ?? '',
        key: a.key ?? '',
        verdict: a.verdict ?? '',
        text: body.trim(),
      },
    })
  }
  return events
}

// whoami answers who this session speaks as, asking irciroh the first time.
async function whoami($: StateDollar & { mcp: { call: (server: string, tool: string) => Promise<unknown> } }): Promise<IrcSelf> {
  const known = await read($, self)
  if (known.nick !== '') return known
  try {
    const out = record(await $.mcp.call('irciroh', 'irc_whoami'))
    const me: IrcSelf = { nick: str(out?.nick), key: str(out?.key) }
    return me.nick === '' ? known : await update($, self, () => me)
  } catch {
    return known
  }
}

export const register: Register = on => {
  on('session.start', async ($, e, next) => {
    void whoami($)
    return next(e)
  })

  // A line pushed as a channel event is kept as it is stored, since its row
  // is handed only the sender's name and the text.
  on('session.append', { origin: { kind: 'channel' } }, async ($, e, next) => {
    if (e.origin.kind !== 'channel' || e.origin.server !== 'irciroh') return next(e)
    const text = e.message.content.map(b => (b.type === 'text' ? b.text : '')).join('\n')
    const events = eventsOf(text, await $.clock.now())
    const last = events.at(-1)
    if (last !== undefined) {
      await update($, pushed, list => [...list, ...events.map(v => v.line)].slice(-MAX_PUSHED))
      await update($, heard, (was): IrcHeard => ({ target: last.target || was.target, last: last.line }))
      void whoami($)
    }
    return next(e)
  }).catch(($, e, next) => next(e))

  on('ui.render', { component: 'UserMessage', props: { origin: { kind: 'channel' } } }, async ($, e, next) => {
    const { origin, text, from, isExpanded } = e.props
    if (origin.kind !== 'channel' || origin.server !== 'irciroh' || isExpanded) return next(e)
    const said = text.trim()
    if (said === '') return next(e)
    const kept = (await read($, pushed)).findLast(l => l.text === said || said.endsWith(l.text))
    return row($.ui.resolve(e), kept ?? {
      time: '', kind: 'message', from: from?.name ?? '', key: '', verdict: '', text: said,
    })
  })

  // Remember who we are, where we last spoke or listened, and what was
  // last heard there, for the rows and the band to draw from.
  on('tool.call', { tool: TOOL }, async ($, e, next) => {
    const kind = verb(String(e.tool))
    const target = str((e as Record_).target)
    if (kind === 'send') {
      await whoami($)
      const now = await $.clock.now()
      await update($, { plugin: 'irc-view', key: 'sent', id: e.tool_use_id } as const, () => now)
    }
    const ran = await next(e)
    if (ran.deny !== undefined || ran.isError === true) return ran
    if (kind === 'whoami') {
      const out = record(ran.result)
      const me: IrcSelf = { nick: str(out?.nick), key: str(out?.key) }
      if (me.nick !== '') await update($, self, () => me)
    }
    if (target !== '' && (kind === 'send' || kind === 'wait' || kind === 'read' || kind === 'join')) {
      const last = kind === 'wait' || kind === 'read' ? linesOf(ran.result)?.at(-1) : undefined
      await update($, heard, (was): IrcHeard => ({
        target,
        last: last ?? (was.target === target ? was.last : null),
      }))
    }
    return ran
  }).catch(($, e, next) => next(e))

  on('ui.render', { component: 'ToolUse', props: { tool: TOOL } }, async ($, e, next) => {
    const { tool, input, isRunning, isErrored, isInterrupted, isExpanded, output } = e.props
    const t = $.ui.resolve(e)
    const head = call(t, tool, input, isRunning, await read($, self), await read($, memberOf(sent, e)))
    if (head === null) return next(e)
    // A row drawn in full (ctrl+o, an unfolded group) has no ToolResult of
    // its own beside it in a group, so the answer is drawn here.
    const body = isExpanded === true && !isRunning && !isErrored && !isInterrupted ? answer(t, tool, output) : null
    return body === null ? head : column(t, [head, body])
  })

  on('ui.render', { component: 'ToolResult', props: { tool: TOOL } }, ($, e, next) => {
    // ctrl+o shows the stored result as the engine draws it.
    if (e.props.isErrored || e.props.isExpanded === true) return next(e)
    return answer($.ui.resolve(e), e.props.tool, e.props.output) ?? next(e)
  })

  // A folded run of calls ("Called irciroh 3 times") hides the talk; unfold
  // any that holds one of ours.
  on('ui.render', { component: 'ToolGroup' }, ($, e, next) =>
    e.props.calls.some(c => verb(c.tool) !== '')
      ? next({ ...e, props: { ...e.props, isExpanded: true } })
      : next(e),
  )

  on('ui.render', { component: 'AbovePrompt' }, async ($, e, next) => {
    const { target, last } = await read($, heard)
    if (target === '' || e.props.hasSurvey) return next(e)
    const me = await read($, self)
    const { Box, Text } = $.ui.resolve(e)
    return (
      <Box width={e.props.bodyColumns}>
        <Box flexShrink={0}>
          <Text weight="bold" color="claude">{target}</Text>
          {me.nick !== '' && (
            <Text dimColor>
              {' '}as <Text color={colorOf(me.nick, me.key)}>{me.nick}</Text>
            </Text>
          )}
          <Text dimColor> │ </Text>
        </Box>
        {last === null ? (
          <Text dimColor>nothing heard yet</Text>
        ) : (
          <Text wrap="truncate-end">
            <Text dimColor>{hhmm(last.time)} </Text>
            {last.from !== '' && <Text color={colorOf(last.from, last.key)}>{`<${last.from}> `}</Text>}
            {last.text}
          </Text>
        )}
      </Box>
    )
  })
}
