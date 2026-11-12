import { expect, test } from 'claude-code/testing'

const KEY = 'a8fdd4c0ffee00112233445566778899'
const LINES = [
  { time: '2026-11-11T23:57:00Z', target: '#pam', kind: 'event', verdict: 'unattributed', text: 'scout joined' },
  { time: '2026-11-11T23:58:00Z', target: '#pam', kind: 'message', from: 'scout', key: KEY, verdict: 'unknown', text: 'lead: Quill.' },
]

// shown is every string of a drawn tree, in order.
function shown(node: unknown): string {
  if (typeof node === 'string') return node
  if (Array.isArray(node)) return node.map(shown).join('')
  if (typeof node !== 'object' || node === null) return ''
  const { children, props } = node as { children?: unknown; props?: { children?: unknown } }
  return shown(children ?? props?.children)
}

const use = (tool: string, input: unknown, more: object = {}) =>
  ({
    plugin: 'irc-view',
    component: 'ToolUse',
    requestId: 'toolu_1',
    props: {
      tool_use_id: 'toolu_1',
      tool: `mcp__irciroh__${tool}`,
      input,
      isRunning: false,
      isErrored: false,
      isInterrupted: false,
      ...more,
    },
  }) as const

const result = (tool: string, output: unknown) =>
  ({
    plugin: 'irc-view',
    component: 'ToolResult',
    requestId: 'toolu_1',
    props: { tool_use_id: 'toolu_1', tool: `mcp__irciroh__${tool}`, output, isErrored: false },
  }) as const

test('a send is the line said, on every surface', async $ => {
  for (const surface of ['terminal', 'desktop'] as const) {
    const ui = await $.ui.mount({ ...use('irc_send', { target: '#pam', text: 'hello\nthere' }), surface })
    expect(shown(await ui.drawn())).toMatch(/<me> hello.*<me> there/)
    await ui.unmount()
  }
})

test('a wait listens, then lists what was heard', async $ => {
  const waiting = await $.ui.mount({
    ...use('irc_wait', { target: '#pam' }, { isRunning: true }),
    surface: 'terminal',
  })
  expect(shown(await waiting.drawn())).toContain('… listening on #pam')
  await waiting.unmount()

  // The result as the structured object, as an MCP envelope, and as text.
  const forms = [
    { matched: true, lines: LINES },
    { content: [{ type: 'text', text: 'x' }], structuredContent: { matched: true, lines: LINES } },
    [{ type: 'text', text: JSON.stringify({ lines: LINES }) }],
    `2026-11-11T23:57:00Z -!- scout joined\n2026-11-11T23:58:00Z <scout[a8fdd4]> lead: Quill.\n`,
  ]
  for (const output of forms) {
    const ui = await $.ui.mount({ ...result('irc_wait', output), surface: 'terminal' })
    const text = shown(await ui.drawn())
    expect(text).toContain('-!- scout joined')
    expect(text).toContain('<scout·a8fdd4> lead: Quill.')
    expect((await ui.find({ type: 'Text', text: /^scout$/ }))?.props.color).toMatch(/^#/)
    await ui.unmount()
  }
})

test('a long read keeps the newest lines', async $ => {
  const lines = Array.from({ length: 11 }, (_, i) => ({ ...LINES[1], text: `line ${i}` }))
  const ui = await $.ui.mount({ ...result('irc_read', { lines }), surface: 'terminal' })
  const text = shown(await ui.drawn())
  expect(text).toContain('+3 earlier')
  expect(text).not.toContain('line 2')
  expect(text).toContain('line 10')
  await ui.unmount()
})

test('a ticket is not drawn', async $ => {
  const ui = await $.ui.mount({
    ...result('irc_ticket', { target: '#pam', ticket: 'SECRETSECRET' }),
    surface: 'terminal',
  })
  expect(shown(await ui.drawn())).not.toContain('SECRET')
  await ui.unmount()
})

test('what does not parse, and any other tool, goes to the engine', async ($, on) => {
  on('ui.render', ($, e) => {
    const { Text } = $.ui.resolve(e)

    return <Text>engine</Text>
  })
  const rows = [
    result('irc_wait', 'not a log'),
    result('irc_send', { nope: 1 }),
    use('irc_send', { target: '#pam' }),
    { ...use('irc_send', { target: '#pam', text: 'hi' }), props: { ...use('x', {}).props, tool: 'Bash', input: { command: 'ls' } } },
  ]
  for (const row of rows) {
    const ui = await $.ui.mount({ ...row, surface: 'terminal' })
    expect(shown(await ui.drawn())).toBe('engine')
    await ui.unmount()
  }
})

test('the band names the channel and the last line heard', async ($, on) => {
  on('tool.call', (_, e) =>
    String(e.tool).endsWith('whoami')
      ? { result: { nick: 'lead', key: KEY, dir: '/x', chatting: true } }
      : { result: { matched: true, lines: LINES } },
  )
  await $.tool.call({ tool: 'mcp__irciroh__irc_whoami' })
  await $.tool.call({ tool: 'mcp__irciroh__irc_wait', target: '#pam' })

  const ui = await $.ui.mount({
    plugin: 'irc-view',
    component: 'AbovePrompt',
    requestId: 'band',
    surface: 'terminal',
    props: {
      hasSurvey: false,
      isCollapsed: false,
      isWorking: false,
      maxRows: 10,
      bodyColumns: 130,
      scroll: { offset: 0, bodyRows: 10 },
      view: {},
    },
  })
  const text = shown(await ui.drawn())
  expect(text).toContain('#pam as lead')
  expect(text).toContain('<scout> lead: Quill.')
  await ui.unmount()
})

test('a pushed line draws as heard, and feeds the band', async ($, on) => {
  on('clock.now', () => ({ value: 0 }))
  on('mcp.call', () => ({ value: { content: [], isError: false, structuredContent: { nick: 'lead', key: KEY } } }))
  const text = 'pilot: are you hearing this as a push event? answer in five words or fewer please'
  await $.session.append({
    door: 'delivery',
    origin: { kind: 'channel', server: 'irciroh' },
    uuid: 'u1',
    message: {
      type: 'user',
      role: 'user',
      content: [{
        type: 'text',
        text: `<channel source="irciroh" target="#pam" from="tmc" key="${KEY}" ts="2026-11-11T23:58:00Z" verdict="unknown">${text}</channel>`,
      }],
    },
  })

  const props = { origin: { kind: 'channel', server: 'irciroh' }, isExpanded: false, from: { name: 'tmc' } } as const
  const ui = await $.ui.mount({
    plugin: 'irc-view', component: 'UserMessage', requestId: 'u1', surface: 'terminal',
    props: { ...props, text },
  })
  expect(shown(await ui.drawn())).toContain(`<tmc·a8fdd4> ${text}`)
  await ui.unmount()

  // A line the mod did not see stored still draws, from the row's own props.
  const bare = await $.ui.mount({
    plugin: 'irc-view', component: 'UserMessage', requestId: 'u2', surface: 'terminal',
    props: { ...props, text: 'unseen' },
  })
  expect(shown(await bare.drawn())).toContain('<tmc> unseen')
  await bare.unmount()

  const band = await $.ui.mount({
    plugin: 'irc-view', component: 'AbovePrompt', requestId: 'band', surface: 'terminal',
    props: {
      hasSurvey: false, isCollapsed: false, isWorking: false, maxRows: 10, bodyColumns: 130,
      scroll: { offset: 0, bodyRows: 10 }, view: {},
    },
  })
  const drawn = shown(await band.drawn())
  expect(drawn).toContain('#pam as lead')
  expect(drawn).toContain('<tmc> pilot: are you hearing')
  await band.unmount()
})
