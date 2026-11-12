import type { On } from 'claude-code'
import { expect, mock, test } from 'claude-code/testing'

// The engine stands beneath the plugin in a test, so every `gtme` call the
// mod makes is answered here from a fake ledger: one run waiting on a person.

const RUN = '01M76GQC5CYXS389QS9MCB2TFM'
const KEY = 'jane@acme.com'

const PANE = {
  plugin: 'gtme',
  component: 'Pane',
  requestId: 'gtme-console',
  props: {
    title: 'gtme',
    isFocused: false,
    bodyColumns: 100,
    placement: 'inline',
    scroll: { offset: 0, bodyRows: 20 },
    view: {},
  },
} as const

const CONFIG = JSON.stringify({
  name: 'review',
  source: { id: 'source', use: 'csv/source' },
  steps: [
    { id: 'approve', use: 'human/filter' },
    { id: 'out', use: 'csv/deliver' },
  ],
})

function fakeGtme(argv: readonly string[], answered: boolean) {
  const args = argv.slice(1)
  const ok = (stdout: string) => ({ exitCode: 0, stdout, stderr: '', isStdoutTruncated: false, isStderrTruncated: false })
  if (args[0] === 'query') {
    const sql = args[1] ?? ''
    if (sql.includes('FROM runs ORDER BY')) {
      return ok(JSON.stringify({ id: RUN, pipeline: 'review', status: 'pending', dry: 0, started_at: '2026-11-11T23:57:37.836Z' }) + '\n')
    }
    if (sql.includes('config_json')) {
      return ok(JSON.stringify({ config_json: CONFIG }) + '\n')
    }
    if (sql.includes("event = 'answered'")) {
      return ok(answered ? JSON.stringify({ identity_key: KEY, step_id: 'approve', detail: JSON.stringify({ fields: { pass: true }, participant: 'human/console' }) }) + '\n' : '')
    }
    if (sql.includes('FROM step_events')) {
      return ok(['{"step_id":"source","event":"done","n":1}', '{"step_id":"approve","event":"claimed","n":1}', '{"step_id":"approve","event":"pending","n":1}'].join('\n'))
    }

    return ok('')
  }
  if (args[0] === 'runs') {
    return ok('total: $0 spent\n')
  }
  if (args[0] === 'show') {
    return ok(JSON.stringify({ identity_key: KEY, step: 'approve', role: 'filter', run_id: RUN, surface: 'full_name: "Jane Doe"\ntitle: "VP Sales"\ndemo.score: 82\n' }) + '\n')
  }

  return ok('')
}

for (const surface of ['terminal', 'desktop'] as const) {
test(`${surface}: Approve answers the record through gtme, from the command's folder and environment`, async ($, on) => {
  const calls: { argv: readonly string[]; cwd?: string; env?: Record<string, string> }[] = []
  let answered = false

  on('process.run', (_, e) => {
    calls.push({ argv: e.argv, cwd: e.init?.cwd, env: e.init?.env })
    if (e.argv[1] === 'answer') {
      answered = true
    }

    return { value: fakeGtme(e.argv, answered) }
  })
  on('fs.exists', () => ({ value: true }))
  on('ui.open', () => ({ value: { isPlaced: true } }))
  on('ui.toast', () => ({ value: undefined }))
  on('ui.notify', () => ({ value: { isSent: true, channel: 'client' as const } }))
  on('tool.call', () => ({ result: {} }))
  mock.clock(on)

  // Claude runs the pipeline; the mod follows its folder and environment.
  await $.tool.call({
    tool: 'Bash',
    command: 'S=/tmp/x; cd $S/work && HOME=$S/home GTME_LEDGER=$S/ledger.db gtme run review.yaml',
  })

  {
    const ui = await $.ui.mount({ ...PANE, surface })

    expect(await ui.find({ type: 'Text', text: /Jane Doe · VP Sales · demo\.score 82/ })).toBeDefined()
    await ui.press({ key: `approve:${KEY}` })

    const answer = calls.find(c => c.argv[1] === 'answer')
    expect(answer?.argv.slice(1)).toEqual([
      'answer', RUN, 'approve', KEY,
      '--set', 'pass=true', '--set', 'reason=approved in the gtme console', '--as', 'console',
    ])
    expect(answer?.cwd).toBe('/tmp/x/work')
    expect(answer?.env).toEqual({ GTME_LEDGER: '/tmp/x/ledger.db', HOME: '/tmp/x/home' })

    // The answered record shows who answered, and the queue offers Collect.
    expect(await ui.find({ type: 'Text', text: /approved by human\/console/ })).toBeDefined()
    expect(await ui.find({ type: 'Button', key: 'collect' })).toBeDefined()
    await ui.unmount()
  }
})
}

test('a command that is not gtme run or answer is left alone', async ($, on) => {
  let ran = 0
  on('process.run', () => {
    ran += 1

    return { value: { exitCode: 0, stdout: '', stderr: '', isStdoutTruncated: false, isStderrTruncated: false } }
  })
  on('tool.call', () => ({ result: {} }))

  await $.tool.call({ tool: 'Bash', command: 'gtme plan review.yaml' })
  expect(ran).toBe(0)
})

test('with the console and the gates off, a gtme run draws nothing and calls nothing', { options: { console: false, gates: false } }, async ($, on) => {
  let ran = 0
  on('process.run', () => {
    ran += 1

    return { value: { exitCode: 0, stdout: '', stderr: '', isStdoutTruncated: false, isStderrTruncated: false } }
  })
  on('tool.call', () => ({ result: {} }))

  await $.tool.call({ tool: 'Bash', command: 'gtme run review.yaml' })
  expect(ran).toBe(0)
})

test('with only the console off, the gates still hold a key', { options: { console: false } }, async ($, on) => {
  let reached = false
  on('tool.call', () => {
    reached = true
    return { result: {} }
  })

  const r = await $.tool.call({ tool: 'Bash', command: ['gtme', 'secret', 'set', 'SOME_KEY', 'some-value'].join(' ') })
  expect(String((r as { deny?: string }).deny)).toMatch(/never put a key on the command line/)
  expect(reached).toBe(false)
})

test('docked beside the transcript, the pane keeps the step rows while records wait', async ($, on) => {
  on('process.run', (_, e) => ({ value: fakeGtme(e.argv, false) }))
  on('fs.exists', () => ({ value: true }))
  on('ui.open', () => ({ value: { isPlaced: true } }))
  on('ui.toast', () => ({ value: undefined }))
  on('ui.notify', () => ({ value: { isSent: true, channel: 'client' as const } }))
  on('tool.call', () => ({ result: {} }))
  mock.clock(on)

  await $.tool.call({ tool: 'Bash', command: 'cd /tmp/x && gtme run review.yaml' })
  const ui = await $.ui.mount({ ...PANE, surface: 'terminal', props: { ...PANE.props, placement: 'dock', bodyColumns: 60, scroll: { offset: 0, bodyRows: 40 } } })

  expect(await ui.find({ type: 'Text', text: /human\/filter/ })).toBeDefined()
  expect(await ui.find({ type: 'Text', text: /Jane Doe · VP Sales/ })).toBeDefined()
  expect(await ui.find({ type: 'Button', key: `approve:${KEY}` })).toBeDefined()
  await ui.unmount()
})

// A copy pipeline: a review grades the draft, then a compose rewrites it.
const COPY_CONFIG = JSON.stringify({
  name: 'copy',
  source: { id: 'source', use: 'csv/source' },
  steps: [
    { id: 'draft', use: 'fake-draft' },
    { id: 'grade', use: 'human/review' },
    { id: 'edit', use: 'human/compose' },
  ],
})
const DRAFT = 'Maya, quick one: how is Northwind handling outbound?'

function fakeCopy(argv: readonly string[], step: 'grade' | 'edit') {
  const args = argv.slice(1)
  const ok = (stdout: string) => ({ exitCode: 0, stdout, stderr: '', isStdoutTruncated: false, isStderrTruncated: false })
  if (args[0] === 'query') {
    const sql = args[1] ?? ''
    if (sql.includes('FROM runs ORDER BY')) {
      return ok(JSON.stringify({ id: RUN, pipeline: 'copy', status: 'pending', dry: 0, started_at: '2026-11-12T03:40:00.000Z' }) + '\n')
    }
    if (sql.includes('config_json')) {
      return ok(JSON.stringify({ config_json: COPY_CONFIG }) + '\n')
    }
    if (sql.includes('FROM step_events') && !sql.includes('answered')) {
      return ok(`{"step_id":"${step}","event":"pending","n":1}\n`)
    }

    return ok('')
  }
  if (args[0] === 'show') {
    const outputs = step === 'grade' ? ['copy.grade=ship|rewrite|drop'] : ['copy.first_line=<text>']
    const surface = `draft.first_line (the value under review): ${JSON.stringify(DRAFT)}\nfull_name: "Maya Chen"\n`
    return ok(JSON.stringify({ identity_key: KEY, step, role: step === 'grade' ? 'review' : 'compose', run_id: RUN, outputs, surface }) + '\n')
  }

  return ok('')
}

function engine(on: On, step: 'grade' | 'edit', calls: (readonly string[])[]) {
  on('process.run', (_, e) => {
    calls.push(e.argv)

    return { value: fakeCopy(e.argv, step) }
  })
  on('fs.exists', () => ({ value: true }))
  on('ui.open', () => ({ value: { isPlaced: true } }))
  on('ui.toast', () => ({ value: undefined }))
  on('ui.notify', () => ({ value: { isSent: true, channel: 'client' as const } }))
  on('tool.call', () => ({ result: {} }))
  mock.clock(on)
}

test('a review shows the draft and answers with the label pressed', async ($, on) => {
  const calls: (readonly string[])[] = []
  engine(on, 'grade', calls)
  await $.tool.call({ tool: 'Bash', command: 'cd /tmp/x && gtme run copy.yaml' })
  const ui = await $.ui.mount({ ...PANE, surface: 'terminal' })

  expect(await ui.find({ type: 'Text', text: /quick one: how is Northwind/ })).toBeDefined()
  await ui.press({ key: `choose:grade/${KEY}:copy.grade:ship` })
  expect(calls.find(a => a[1] === 'answer')?.slice(1)).toEqual(['answer', RUN, 'grade', KEY, '--set', 'copy.grade=ship', '--as', 'console'])
  await ui.unmount()
})

test('a compose of: starts from the draft and saves what the person wrote', async ($, on) => {
  const calls: (readonly string[])[] = []
  engine(on, 'edit', calls)
  await $.tool.call({ tool: 'Bash', command: 'cd /tmp/x && gtme run copy.yaml' })
  const ui = await $.ui.mount({ ...PANE, surface: 'terminal' })

  const field = await ui.find({ type: 'Input', key: `write:edit/${KEY}:copy.first_line` })
  expect(field?.props.value).toBe(DRAFT)
  await ui.input({ key: `write:edit/${KEY}:copy.first_line`, text: 'Maya, how is Northwind finding reps right now?' })
  expect(calls.find(a => a[1] === 'answer')?.slice(1)).toEqual([
    'answer', RUN, 'edit', KEY, '--set', 'copy.first_line=Maya, how is Northwind finding reps right now?', '--as', 'console',
  ])
  await ui.unmount()
})

test('Draft with Claude fills the field, and saving it notes who drafted it', async ($, on) => {
  const calls: (readonly string[])[] = []
  engine(on, 'edit', calls)
  on('model.complete', () => ({ value: { isAnswered: true as const, text: 'Maya, what does outbound look like at Northwind this quarter?', usage: { input_tokens: 1, output_tokens: 1, cache_read_input_tokens: 0, cache_creation_input_tokens: 0 } } }))
  await $.tool.call({ tool: 'Bash', command: 'cd /tmp/x && gtme run copy.yaml' })
  const ui = await $.ui.mount({ ...PANE, surface: 'terminal' })

  await ui.press({ key: `draft:edit/${KEY}` })
  const field = await ui.find({ type: 'Input', key: `write:edit/${KEY}:copy.first_line` })
  expect(field?.props.value).toBe('Maya, what does outbound look like at Northwind this quarter?')
  await ui.press({ key: `save:edit/${KEY}` })
  const answer = calls.find(a => a[1] === 'answer')
  expect(answer?.slice(-2)).toEqual(['--note', 'drafted with Claude in the gtme console'])
  await ui.unmount()
})

// The gates. `gtme plan` is answered from text shaped like the real one.
const PLAN_VENDOR = `pipeline send (version 1)

send surface: 2 deliver step(s)
  send → http/deliver (touch scope: send)
  log → csv/deliver (touch scope: send)

plan ok — nothing has been spent
`
const PLAN_LOCAL = `pipeline local (version 1)

send surface: 1 deliver step(s)
  out → csv/deliver (touch scope: local)

plan ok — nothing has been spent
`

function gateEngine(on: On, opts: { plan: string; dryRun: boolean; choice?: string }) {
  const seen = { ran: false, asked: '' }
  const ok = (stdout: string) => ({ exitCode: 0, stdout: '', stderr: stdout, isStdoutTruncated: false, isStderrTruncated: false })
  on('process.run', (_, e) => {
    const args = e.argv.slice(1)
    if (args[0] === 'plan') {
      return { value: ok(opts.plan) }
    }
    if (args[0] === 'query' && (args[1] ?? '').includes('dry = 1')) {
      return { value: { ...ok(''), stdout: opts.dryRun ? `{"id":"${RUN}","started_at":"2026-11-12T04:00:00.000Z"}\n` : '' } }
    }
    if (args[0] === 'query' && (args[1] ?? '').includes("event = 'dry_run'")) {
      return { value: { ...ok(''), stdout: '{"n":3}\n' } }
    }

    return { value: ok('') }
  })
  on('fs.stat', () => ({ value: { kind: 'file' as const, size: 1, mtimeMs: 0, isLink: false } }))
  on('fs.exists', () => ({ value: true }))
  on('ui.open', () => ({ value: { isPlaced: true } }))
  on('ui.toast', () => ({ value: undefined }))
  on('ui.notify', () => ({ value: { isSent: true, channel: 'client' as const } }))
  on('tool.call', (_, e) => {
    if (e.tool === 'AskUserQuestion') {
      const question = e.questions[0]?.question ?? ''
      seen.asked = question
      return { result: { questions: e.questions, answers: { [question]: opts.choice ?? 'Arm it' } } }
    }
    seen.ran = true
    return { result: {} }
  })
  mock.clock(on)

  return seen
}

test('a key on the command line is held', async ($, on) => {
  const seen = gateEngine(on, { plan: PLAN_LOCAL, dryRun: false })
  const r = await $.tool.call({ tool: 'Bash', command: 'gtme secret set INSTANTLY_API_KEY sk_123' })
  expect(String((r as { deny?: string }).deny)).toMatch(/never put a key on the command line/)
  expect(seen.ran).toBe(false)
})

test('an armed run to a vendor with no dry run is held', async ($, on) => {
  const seen = gateEngine(on, { plan: PLAN_VENDOR, dryRun: false })
  const r = await $.tool.call({ tool: 'Bash', command: 'cd /tmp/x && gtme run send.yaml' })
  expect(String((r as { deny?: string }).deny)).toMatch(/--dry-run/)
  expect(seen.ran).toBe(false)
})

test('after a dry run, the person arms it', async ($, on) => {
  const seen = gateEngine(on, { plan: PLAN_VENDOR, dryRun: true, choice: 'Arm it' })
  const r = await $.tool.call({ tool: 'Bash', command: 'cd /tmp/x && gtme run send.yaml' })
  expect((r as { deny?: string }).deny).toBeUndefined()
  expect(seen.ran).toBe(true)
})

test('after a dry run, Not yet holds it', async ($, on) => {
  const seen = gateEngine(on, { plan: PLAN_VENDOR, dryRun: true, choice: 'Not yet' })
  const r = await $.tool.call({ tool: 'Bash', command: 'cd /tmp/x && gtme run send.yaml' })
  expect(String((r as { deny?: string }).deny)).toMatch(/did not arm send/)
  expect(seen.ran).toBe(false)
})

test('a dry run, and a pipeline that only writes locally, pass with no question', async ($, on) => {
  const seen = gateEngine(on, { plan: PLAN_LOCAL, dryRun: false })
  await $.tool.call({ tool: 'Bash', command: 'cd /tmp/x && gtme run send.yaml --dry-run' })
  expect(seen.ran).toBe(true)
  seen.ran = false
  await $.tool.call({ tool: 'Bash', command: 'cd /tmp/x && gtme run local.yaml' })
  expect(seen.ran).toBe(true)
  expect(seen.asked).toBe('')
})

test('prose that mentions gtme is not a gtme command', async ($, on) => {
  const seen = gateEngine(on, { plan: PLAN_VENDOR, dryRun: false })
  // A heredoc whose text names both commands: the shell runs `cat`, not gtme.
  await $.tool.call({
    tool: 'Bash',
    command: "cat > notes.md <<'EOF'\n\"description\": \"an armed gtme run waits, and gtme secret set with the value is refused\"\ngtme run send.yaml is how it starts\nEOF",
  })
  expect(seen.ran).toBe(true)
  expect(seen.asked).toBe('')
})

test('with the gates setting off, nothing is held', { options: { gates: false } }, async ($, on) => {
  const seen = gateEngine(on, { plan: PLAN_VENDOR, dryRun: false })
  await $.tool.call({ tool: 'Bash', command: 'gtme secret set INSTANTLY_API_KEY sk_123' })
  expect(seen.ran).toBe(true)
})

test('before any command names a ledger, the header names the one the environment sets', async ($, on) => {
  on('env.get', (_, e) => ({ value: e.name === 'GTME_LEDGER' ? '/work/demo/demo.db' : undefined }))
  on('process.run', () => ({ value: { exitCode: 0, stdout: '', stderr: '', isStdoutTruncated: false, isStderrTruncated: false } }))
  const ui = await $.ui.mount({ ...PANE, surface: 'terminal' })
  expect(await ui.find({ type: 'Text', text: /…\/demo\/demo\.db/ })).toBeDefined()
  await ui.unmount()
})
