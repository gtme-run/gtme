import { atom, read, update } from 'claude-code'
import type { EngineInterface, Register } from 'claude-code'

import type {
  Active,
  CostRow,
  Drafts,
  PlanStep,
  ReviewItem,
  RunRow,
  Snapshot,
  StepCount,
} from '../types'

// The gtme console (DECISIONS.md ADR-067). Every fact comes from the gtme binary (`gtme query`, `gtme runs`,
// `gtme show --pending`); the one write is `gtme answer`, pressed by the
// person. The mod never opens the ledger itself.

const PANE = 'gtme-console'
// The width the console asks for when the surface docks it beside a
// fullscreen transcript (Claude Code caps a dock at 45% of the terminal).
const DOCK_COLUMNS = 64
const BINARIES = ['gtme', '/opt/homebrew/bin/gtme', '/usr/local/bin/gtme']
const ULID = /^[0-9A-HJKMNP-TV-Z]{26}$/
const GTME_VERB = /(^|[\s;&|(])gtme\s+(run|answer)\b/
const FINISHED = ['done', 'failed', 'skipped_cache', 'dry_run', 'gated', 'pending', 'collected', 'settled']
const HEADER = /^(run|pipeline|status|started|finished|records|config|step)\b/

const EMPTY: Snapshot = {
  ledger: null,
  env: {},
  workdir: null,
  runs: [],
  plan: [],
  steps: [],
  costs: [],
  receipt: '',
  review: [],
  pending: 0,
  error: null,
  refreshedAt: 0,
}

// The tag declines a snapshot an earlier version stored (it outlives a reload).
const snap = atom({ plugin: 'gtme', key: 'snap' } as const, EMPTY, { shape: 'snap-4' })
const active = atom({ plugin: 'gtme', key: 'active' } as const, null)
const drafts = atom({ plugin: 'gtme', key: 'drafts' } as const, {} as Drafts)
// `step/key` of the record Claude is drafting for, while it does.
const drafting = atom({ plugin: 'gtme', key: 'drafting' } as const, null)

// The model the console's Draft with Claude asks: a short line needs no more.
const DRAFT_MODEL = 'haiku'

let isRefreshing = false

// ---------------------------------------------------------------- the binary

type Where = { env: Record<string, string>; workdir: string | null }

// The environment a gtme command sets that changes what gtme reads: which
// ledger, and which home its installed adapters are found under.
const FOLLOWED = ['GTME_LEDGER', 'GTME_HOME', 'HOME'] as const

async function gtme($: EngineInterface, args: string[], where: Where) {
  for (const bin of BINARIES) {
    try {
      return await $.process.run([bin, ...args], {
        env: Object.keys(where.env).length === 0 ? undefined : where.env,
        // gtme re-plans a run's recorded pipeline to list or answer what is
        // pending, and that resolves the source path from the working
        // directory and adapters from HOME: run it as the command did.
        cwd: where.workdir ?? undefined,
        timeoutMs: 15000,
      })
    } catch {
      // not at this path; try the next
    }
  }

  return null
}

function ndjson<T>(text: string): T[] {
  return text
    .split('\n')
    .filter(line => line.startsWith('{'))
    .map(line => JSON.parse(line) as T)
}

async function query<T>($: EngineInterface, sql: string, where: Where) {
  const ran = await gtme($, ['query', sql, '--format', 'ndjson'], where)
  if (ran === null) {
    throw new Error('gtme not found on PATH (brew install gtme-run/tap/gtme)')
  }
  if (ran.exitCode !== 0) {
    throw new Error(ran.stderr.trim().split('\n')[0] || 'gtme query failed')
  }

  return ndjson<T>(ran.stdout)
}

function planOf(configJson: string): PlanStep[] {
  try {
    const config = JSON.parse(configJson) as {
      source?: { id?: string; use?: string }
      steps?: { id?: string; use?: string }[]
    }
    const source = config.source === undefined ? [] : [{ id: config.source.id ?? 'source', use: config.source.use ?? '' }]
    const steps = (config.steps ?? []).map(s => ({ id: s.id ?? '?', use: s.use ?? '' }))

    return [...source, ...steps]
  } catch {
    return []
  }
}

function answerText(detail: string): string {
  try {
    const parsed = JSON.parse(detail) as { fields?: Record<string, unknown>; participant?: string }
    const fields = parsed.fields ?? {}
    const who = parsed.participant ?? 'someone'
    if (fields.pass === true) {
      return `approved by ${who}`
    }
    if (fields.pass === false) {
      return `rejected by ${who}${typeof fields.reason === 'string' ? ` (${fields.reason})` : ''}`
    }
    const values = Object.values(fields).filter(v => typeof v === 'string' || typeof v === 'number')
    if (values.length > 0) {
      return `${values.map(v => (String(v).includes(' ') ? `“${String(v)}”` : String(v))).join(' · ')} · by ${who}`
    }

    return `answered by ${who}`
  } catch {
    return 'answered'
  }
}

// `draft.first_line (the value under review): "Hi Jane…"`: the line gtme's
// surface gives the value a review or a compose `of:` is about.
const REFERENT_LINE = /^[\w.]+ \(the value[^)]*\): (".*")$/

function referentOf(surface: string): string | null {
  for (const line of surface.split('\n')) {
    const quoted = line.match(REFERENT_LINE)?.[1]
    if (quoted !== undefined) {
      try {
        return JSON.parse(quoted) as string
      } catch {
        return quoted.slice(1, -1)
      }
    }
  }

  return null
}

async function reviewOf($: EngineInterface, run: RunRow, where: Where): Promise<ReviewItem[]> {
  const shown = await gtme($, ['show', '--run', run.id, '--pending'], where)
  if (shown === null) {
    return []
  }
  const waiting = ndjson<{ identity_key: string; step: string; role: string; surface: string; outputs?: string[] }>(shown.stdout)
  if (waiting.length === 0) {
    // gtme printed why on stderr (it could not rebuild the run's plan from
    // here, say): say so rather than show an empty queue.
    const why = shown.stderr.trim().split('\n')[0]
    if (why !== undefined && why !== '' && !why.includes('nothing pending')) {
      throw new Error(`can't list what's waiting: ${why}`)
    }

    return []
  }
  const answered = await query<{ identity_key: string; step_id: string; detail: string }>(
    $,
    `SELECT i.identity_key, e.step_id, e.detail FROM step_events e JOIN identities i ON i.id = e.identity_id
     WHERE e.run_id = '${run.id}' AND e.event = 'answered' ORDER BY e.created_at`,
    where,
  )
  const answers = new Map<string, string>()
  for (const a of answered) {
    answers.set(`${a.step_id}/${a.identity_key}`, answerText(a.detail))
  }

  return waiting.map(w => ({
    key: w.identity_key,
    step: w.step,
    role: w.role,
    referent: referentOf(w.surface),
    outputs: w.outputs ?? [],
    // `full_name: "Jane Doe"` lines → `Jane Doe · VP Sales · demo.score 54`:
    // a plain field shows its value; a namespaced one keeps its name, since a
    // bare number says nothing. The referent's line is drawn on its own.
    surface: w.surface
      .trim()
      .split('\n')
      .filter(line => !REFERENT_LINE.test(line))
      .map(line => {
        const m = line.match(/^([\w.]+): "?(.*?)"?$/)
        if (m === null || m[1] === undefined || m[2] === undefined) {
          return line
        }

        return m[1].includes('.') ? `${m[1]} ${m[2]}` : m[2]
      })
      .join(' · '),
    answer: answers.get(`${w.step}/${w.identity_key}`) ?? null,
  }))
}

async function refresh($: EngineInterface) {
  if (isRefreshing) {
    return
  }
  isRefreshing = true
  const old = await read($, snap)
  const where: Where = { env: old.env, workdir: old.workdir }

  try {
    const runs = await query<RunRow>(
      $,
      'SELECT id, pipeline, status, dry, started_at FROM runs ORDER BY started_at DESC LIMIT 4',
      where,
    )
    const latest = runs[0]
    let plan: PlanStep[] = []
    let steps: StepCount[] = []
    let costs: CostRow[] = []
    let receipt = ''
    let review: ReviewItem[] = []

    if (latest !== undefined && ULID.test(latest.id)) {
      const config = await query<{ config_json: string }>($, `SELECT config_json FROM runs WHERE id = '${latest.id}'`, where)
      plan = planOf(config[0]?.config_json ?? '{}')
      steps = await query<StepCount>(
        $,
        `SELECT step_id, event, count(*) AS n FROM step_events WHERE run_id = '${latest.id}' GROUP BY step_id, event`,
        where,
      )
      costs = await query<CostRow>(
        $,
        `SELECT basis, round(sum(amount_usd), 4) AS usd FROM costs WHERE run_id = '${latest.id}' GROUP BY basis`,
        where,
      )
      if (latest.status !== 'running') {
        const shown = await gtme($, ['runs', latest.id], where)
        receipt = shown === null ? '' : (shown.stdout + shown.stderr).trim()
      }
      if (latest.status === 'pending') {
        review = await reviewOf($, latest, where)
      }
    }

    await update($, snap, s => ({
      ...s,
      runs,
      plan,
      steps,
      costs,
      receipt,
      review,
      pending: review.filter(r => r.answer === null).length,
      error: null,
      refreshedAt: Date.now(),
    }))
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err)
    await update($, snap, s => ({ ...s, error: message, refreshedAt: Date.now() }))
  } finally {
    isRefreshing = false
  }
}

// One participant's answer for one waiting record, through `gtme answer`.
async function answerWith($: EngineInterface, run: RunRow, item: ReviewItem, sets: Record<string, string>, note?: string) {
  const s = await read($, snap)
  const args = ['answer', run.id, item.step, item.key]
  for (const [field, value] of Object.entries(sets)) {
    args.push('--set', `${field}=${value}`)
  }
  args.push('--as', 'console')
  if (note !== undefined) {
    args.push('--note', note)
  }
  const ran = await gtme($, args, { env: s.env, workdir: s.workdir })
  if (ran === null || ran.exitCode !== 0) {
    $.ui.toast(`gtme answer failed: ${ran?.stderr.trim().split('\n')[0] ?? 'gtme not found'}`)
  } else {
    await update($, drafts, d => {
      const { [`${item.step}/${item.key}`]: _done, ...rest } = d
      return rest
    })
  }
  await refresh($)
}

async function answer($: EngineInterface, run: RunRow, item: ReviewItem, pass: boolean) {
  const reason = pass ? 'approved in the gtme console' : 'rejected in the gtme console'
  await answerWith($, run, item, { pass: String(pass), reason })
}

// What a record's outputs ask for: a field with a closed set of labels is a
// choice (one button each); any other is text (an input). A filter's
// pass/reason are its Approve and Reject.
type Control = { field: string; options: string[] }

function controlsOf(item: ReviewItem): Control[] {
  if (item.role === 'filter') {
    return []
  }

  return item.outputs.flatMap(output => {
    const at = output.indexOf('=')
    if (at < 1) {
      return []
    }
    const hint = output.slice(at + 1)
    const options = !hint.startsWith('<') && hint.includes('|') ? hint.split('|') : []

    return [{ field: output.slice(0, at), options }]
  })
}

// The text a compose field shows: what the person typed, else the referent
// it revises, else nothing.
function draftOf(all: Drafts, item: ReviewItem, field: string): string {
  return all[`${item.step}/${item.key}`]?.[field] ?? item.referent ?? ''
}

// Saves every text field of a compose record, with a note when Claude
// drafted the text (and whether the person changed it).
async function saveText($: EngineInterface, run: RunRow, item: ReviewItem, override?: { field: string; value: string }) {
  const all = await read($, drafts)
  const mine = all[`${item.step}/${item.key}`] ?? {}
  const sets: Record<string, string> = {}
  for (const c of controlsOf(item).filter(c => c.options.length === 0)) {
    sets[c.field] = override?.field === c.field ? override.value : draftOf(all, item, c.field)
  }
  if (Object.values(sets).some(v => v.trim() === '')) {
    $.ui.toast('Write something first, or press Draft with Claude.')
    return
  }
  const ai = mine.__ai
  const note = ai === undefined ? undefined : Object.values(sets).includes(ai) ? 'drafted with Claude in the gtme console' : 'drafted with Claude, edited in the gtme console'
  await answerWith($, run, item, sets, note)
}

async function draftWithClaude($: EngineInterface, item: ReviewItem, field: string) {
  const id = `${item.step}/${item.key}`
  await update($, drafting, () => id)
  const r = await $.model.complete({
    model: DRAFT_MODEL,
    maxTokens: 300,
    system:
      'You write outbound sales email copy for a person to review. Reply with the text alone: no quotes, no preamble, no options.',
    prompt: [
      `Write the field "${field}" for this prospect.`,
      item.surface === '' ? '' : `What the reviewer sees: ${item.surface}`,
      item.referent === null ? '' : `The current draft: ${item.referent}\nImprove it.`,
      'Use only facts in what the reviewer sees: drop any claim about them it does not support, and name their company if it is there.',
      'One sentence, under 25 words, specific to them, plain words, no hype.',
    ]
      .filter(line => line !== '')
      .join('\n'),
  })
  await update($, drafting, () => null)
  if (!r.isAnswered) {
    $.ui.toast(`Claude could not draft that line (${r.reason})`)
    return
  }
  const text = r.text.trim()
  await update($, drafts, d => ({ ...d, [id]: { ...(d[id] ?? {}), [field]: text, __ai: text } }))
}

function askText(item: ReviewItem, run: RunRow): string {
  const what =
    item.role === 'filter'
      ? 'recommend approve or reject'
      : item.role === 'review'
        ? `recommend which answer (${item.outputs.join(', ')}) and why`
        : `suggest what to write for ${item.outputs.map(o => o.split('=')[0]).join(', ')}`
  const referent = item.referent === null ? '' : ` The value it is about: “${item.referent}”.`

  return `In the gtme console, record ${item.key} is waiting at step "${item.step}" (${item.role}) of run ${run.id} (pipeline ${run.pipeline}).${item.surface === '' ? '' : ` What it shows: ${item.surface}.`}${referent} Read what the ledger knows about it (gtme show ${item.key} --provenance, in the same environment as the last gtme command) and ${what}, in two sentences. Don't answer it yourself.`
}

// ------------------------------------------------------- reading the command

// The shell assignments a command makes (`S=/tmp/x; GTME_LEDGER=$S/l.db`),
// each value expanded against the ones before it.
function shellVars(command: string): Map<string, string> {
  const vars = new Map<string, string>()
  const assign = /(?:^|[\s;&|(])([A-Za-z_][A-Za-z0-9_]*)=("[^"]*"|'[^']*'|[^\s;&|]+)/g
  for (const m of command.matchAll(assign)) {
    if (m[1] !== undefined && m[2] !== undefined) {
      vars.set(m[1], expand(m[2].replace(/^["']|["']$/g, ''), vars))
    }
  }

  return vars
}

function expand(value: string, vars: Map<string, string>): string {
  return value.replace(/\$\{?([A-Za-z_][A-Za-z0-9_]*)\}?/g, (all, v: string) => vars.get(v) ?? all)
}

function isAbsolute(path: string): boolean {
  return path.startsWith('/') && !path.includes('$')
}

// The followed variables a command sets: { name: path } for each it sets to
// a path we can resolve, and the names it sets to one we cannot.
function envOf(command: string): { env: Record<string, string>; unresolved: string[] } {
  const vars = shellVars(command)
  const env: Record<string, string> = {}
  const unresolved: string[] = []
  for (const name of FOLLOWED) {
    const value = vars.get(name)
    if (value === undefined) {
      continue
    }
    if (isAbsolute(value)) {
      env[name] = value
    } else {
      unresolved.push(name)
    }
  }

  return { env, unresolved }
}

// The last `cd DIR` before the gtme verb, expanded; null when there is none
// we can resolve.
function workdirOf(command: string): string | null {
  const vars = shellVars(command)
  const before = command.slice(0, command.search(GTME_VERB) + 1)
  const dirs = [...before.matchAll(/(?:^|[\s;&|(])cd\s+("[^"]*"|'[^']*'|[^\s;&|]+)/g)]
  const last = dirs.at(-1)?.[1]
  if (last === undefined) {
    return null
  }
  const dir = expand(last.replace(/^["']|["']$/g, ''), vars)

  return isAbsolute(dir) ? dir : null
}

function modeOf(command: string): Active['mode'] {
  if (/\bgtme\s+answer\b/.test(command)) {
    return 'answer'
  }
  if (command.includes('--simulate')) {
    return 'simulate'
  }
  if (command.includes('--dry-run')) {
    return 'dry-run'
  }

  return 'armed'
}

function pipelineOf(command: string): string {
  const match = command.match(/gtme\s+(?:run|answer)\s+([^\s-][^\s]*)/)

  return match?.[1] ?? '(pipeline)'
}

// ------------------------------------------------------------- what to draw

type StepView = {
  id: string
  use: string
  state: 'waiting' | 'running' | 'done' | 'failed' | 'pending'
  detail: string
}

// The receipt's step table, by step id: the counts a person reads.
function receiptRows(receipt: string, ids: string[]): Map<string, string> {
  const rows = new Map<string, string>()
  for (const line of receipt.split('\n')) {
    const cells = line.trim().split(/\s+/)
    const [id, , input, out, , cached, filtered, failed, cost] = cells
    if (id === undefined || !ids.includes(id) || cells.length < 10 || out === undefined) {
      continue
    }
    const parts = [id === ids[0] ? `${out} sourced` : `${input} in → ${out} out`]
    if (cached !== undefined && cached !== '0' && cached !== '-') parts.push(`${cached} cached`)
    if (filtered !== undefined && filtered !== '0' && filtered !== '-') parts.push(`${filtered} filtered`)
    if (failed !== undefined && failed !== '0' && failed !== '-') parts.push(`${failed} failed`)
    if (cost !== undefined && cost !== '$0') parts.push(cost)
    rows.set(id, parts.join(' · '))
  }

  return rows
}

function stepViews(s: Snapshot, runStatus: string): StepView[] {
  const counts = new Map<string, Map<string, number>>()
  for (const row of s.steps) {
    const one = counts.get(row.step_id) ?? new Map<string, number>()
    one.set(row.event, row.n)
    counts.set(row.step_id, one)
  }
  const ids = [...s.plan.map(p => p.id), ...[...counts.keys()].filter(id => !s.plan.some(p => p.id === id))]
  const fromReceipt = receiptRows(s.receipt, ids)

  return ids.map(id => {
    const use = s.plan.find(p => p.id === id)?.use ?? ''
    const events = counts.get(id)
    const receipt = fromReceipt.get(id)
    if (events === undefined) {
      return { id, use, state: runStatus === 'running' ? 'waiting' : 'done', detail: receipt ?? (runStatus === 'running' ? 'waiting' : 'nothing reached it') }
    }
    const claimed = events.get('claimed') ?? 0
    const finished = FINISHED.reduce((sum, ev) => sum + (events.get(ev) ?? 0), 0)
    if (claimed > 0 && finished < claimed && runStatus === 'running') {
      return { id, use, state: 'running', detail: `${claimed - finished} in flight` }
    }
    // Waiting on a person only while records in the queue wait here: a step
    // whose answers were collected keeps its old pending events.
    const mine = s.review.filter(r => r.step === id)
    if ((events.get('pending') ?? 0) > 0 && runStatus === 'pending' && mine.length > 0) {
      const open = mine.filter(r => r.answer === null).length
      const detail =
        open === 0
            ? `${mine.length} answered · collect to continue`
            : `${open} waiting on you${mine.length > open ? ` · ${mine.length - open} answered` : ''}`

      return { id, use, state: 'pending', detail }
    }
    const fallback = [...events].filter(([ev]) => ev !== 'claimed').map(([ev, n]) => `${n} ${ev.replace('_', ' ')}`).join(' · ')

    return { id, use, state: (events.get('failed') ?? 0) > 0 ? 'failed' : 'done', detail: receipt ?? fallback }
  })
}

// The receipt lines a person needs once the table is drawn as rows: the total
// and each step's note (what waits, what was held, what to run next).
function receiptNotes(receipt: string, ids: string[]): string[] {
  return receipt
    .split('\n')
    .map(line => line.trim())
    .filter(line => line.startsWith('total:') || (/^[\w-]+: /.test(line) && !HEADER.test(line) && ids.includes(line.split(':')[0] ?? '')))
}

function ledgerLabel(ledger: string | null): string {
  if (ledger === null) {
    return '~/.gtme/ledger.db'
  }
  const parts = ledger.split('/')

  return `…/${parts.slice(-2).join('/')}`
}

const GLYPH = { waiting: '○', running: '●', done: '✓', failed: '✗', pending: '◆' } as const
const COLOR = { waiting: 'gray', running: 'yellow', done: 'green', failed: 'red', pending: 'magenta' } as const

function spend(costs: CostRow[]): string {
  if (costs.length === 0) {
    return '$0'
  }

  return costs.map(c => `$${c.usd.toFixed(2)} ${c.basis}`).join(' · ')
}

function seconds(fromMs: number): number {
  return Math.max(0, Math.round((Date.now() - fromMs) / 1000))
}

// ------------------------------------------------------------------- gates

// The two rules the skills carry in prose, held here so Claude Code enforces
// them (DECISIONS.md, the 2026-11-11 gates entry): an armed run that reaches
// a vendor waits for the person's yes after a dry run of that pipeline, and
// a key never goes on a command line.

// Deliver adapters whose target is local: a file, or a group in the ledger.
const LOCAL_TARGETS = new Set(['csv/deliver', 'group/deliver'])

// A segment runs gtme only when gtme is its command: at its start, after any
// `VAR=value` assignments. Prose that mentions gtme (a heredoc, an echo, a
// commit message) is not a gtme command, and the gates leave it alone.
const GTME_RUN = /^(?:[A-Za-z_][A-Za-z0-9_]*=\S*\s+)*gtme\s+run\s+([^\s-][^\s]*)/
const GTME_SECRET_SET = /^(?:[A-Za-z_][A-Za-z0-9_]*=\S*\s+)*gtme\s+secret\s+set\s+(\S+)\s+(\S+)/

// The command's segments, split where the shell would run them one by one.
// A heredoc's body is text the shell hands to a command, never runs: it is
// dropped first, from `<<WORD` (or `<<'WORD'`, `<<-WORD`) to the line WORD.
function segmentsOf(command: string): string[] {
  const code = command.replace(/<<-?\s*(['"]?)([A-Za-z_][A-Za-z0-9_]*)\1[^\n]*\n[\s\S]*?\n\s*\2\s*(?=\n|$)/g, '')

  return code.split(/&&|\|\||[;\n|&]/).map(part => part.trim()).filter(part => part !== '')
}

// Each `gtme run PIPELINE` in the command that would send for real: neither
// --dry-run nor --simulate.
function armedRunsOf(command: string): string[] {
  return segmentsOf(command).flatMap(segment => {
    const m = segment.match(GTME_RUN)
    if (m?.[1] === undefined || /\s--(dry-run|simulate)\b/.test(segment)) {
      return []
    }

    return [m[1]]
  })
}

// `gtme secret set KEY VALUE`: the value is on the command line.
function keyOnCommandLine(command: string): string | null {
  for (const segment of segmentsOf(command)) {
    const m = segment.match(GTME_SECRET_SET)
    if (m?.[1] !== undefined) {
      return m[1]
    }
  }

  return null
}

// What `gtme plan` says a pipeline sends to: its name and the deliver steps
// whose adapters are not local. null when plan fails, so the run fails on its
// own and with gtme's own words.
function sendSurfaceOf(planText: string): { name: string; targets: string[] } | null {
  const name = planText.match(/^pipeline (\S+) \(version/m)?.[1]
  if (name === undefined) {
    return null
  }
  const at = planText.indexOf('send surface:')
  if (at === -1) {
    return { name, targets: [] }
  }
  const targets: string[] = []
  for (const line of planText.slice(at).split('\n').slice(1)) {
    const m = line.match(/^\s+\S+ → (\S+)/)
    if (m?.[1] === undefined) {
      break
    }
    if (!LOCAL_TARGETS.has(m[1]) && !targets.includes(m[1])) {
      targets.push(m[1])
    }
  }

  return { name, targets }
}

// Why the command may not run, or null when it may.
async function armGate($: EngineInterface, command: string): Promise<string | null> {
  const runs = armedRunsOf(command)
  if (runs.length === 0) {
    return null
  }
  const where: Where = { env: envOf(command).env, workdir: workdirOf(command) }
  for (const path of runs) {
    const plan = await gtme($, ['plan', path], where)
    if (plan === null) {
      return 'gtme console: gtme is not on PATH, so the arm gate cannot check this run (brew install gtme-run/tap/gtme)'
    }
    const surface = plan.exitCode === 0 ? sendSurfaceOf(plan.stdout + plan.stderr) : null
    if (surface === null || surface.targets.length === 0) {
      continue
    }
    const targets = surface.targets.join(', ')
    const file = path.startsWith('/') || where.workdir === null ? path : `${where.workdir}/${path}`
    let editedAt = 0
    try {
      editedAt = (await $.fs.stat(file)).mtimeMs
    } catch {
      // unreadable here: any dry run of the pipeline counts
    }
    const dry = (
      await query<{ id: string; started_at: string }>(
        $,
        `SELECT id, started_at FROM runs WHERE pipeline = '${surface.name.replace(/'/g, "''")}' AND dry = 1 ORDER BY started_at DESC LIMIT 1`,
        where,
      )
    )[0]
    if (dry === undefined || !ULID.test(dry.id) || Date.parse(dry.started_at) < editedAt) {
      $.ui.toast(`${surface.name}: held an armed run to ${targets} until a dry run`)
      return `gtme console: ${path} sends to ${targets}, and an armed run waits for the person. Run \`gtme run ${path} --dry-run\` first (it sends nothing) and show the person its receipt; then run it armed again and the console asks them to arm it. ${dry === undefined ? 'There is no dry run of this pipeline yet.' : 'The pipeline file changed after its last dry run.'}`
    }
    const resolved = (
      await query<{ n: number }>($, `SELECT count(*) AS n FROM step_events WHERE run_id = '${dry.id}' AND event = 'dry_run'`, where)
    )[0]?.n ?? 0
    let choice: string
    try {
      choice = await $.ui.ask(
        `Arm ${surface.name}? It sends to ${targets}. Its dry run at ${dry.started_at.slice(11, 16)} UTC resolved ${resolved} send(s); its receipt is the last one ${surface.name} printed.`,
        { header: 'Arm gtme', options: ['Arm it', 'Not yet'] },
      )
    } catch {
      return `gtme console: the person closed the arm dialog for ${surface.name} without arming it, so the run did not start`
    }
    if (choice !== 'Arm it') {
      return `gtme console: the person did not arm ${surface.name}${choice === 'Not yet' ? '' : `; they said: “${choice}”`}`
    }
  }

  return null
}

// ------------------------------------------------------------------- hooks

export const register: Register = (on, options) => {
  // The gates come first, so they hold a command before anything else sees
  // it run, and stand whether or not the console is shown: the plugin's
  // `gates` setting (userConfig, default on) is theirs alone.
  if (options.gates !== false) {
    on('tool.call', { tool: 'Bash' }, async ($, e, next) => {
      if (!/\bgtme\s+(run|secret)\b/.test(e.command)) {
        return next(e)
      }
      const key = keyOnCommandLine(e.command)
      if (key !== null) {
        $.ui.toast(`held a command that put ${key}'s value on the command line`)
        return {
          deny: `gtme console: never put a key on the command line. Ask the person to run \`gtme secret set ${key}\` themselves (in Claude Code, \`! gtme secret set ${key}\`), which prompts for the value without echoing it.`,
        }
      }
      const held = await armGate($, e.command)

      return held === null ? next(e) : { deny: held }
    }).catch(($, e, next) =>
      next.called ? next(e) : { deny: 'gtme console: the arm gate failed while checking this command, so it held it; try again, or turn the gates off in /plugin' },
    )
  }

  // The plugin's `console` setting (userConfig, default on). Off, the console
  // registers nothing more and the skills run as they always have.
  if (options.console === false) {
    return
  }

  on('session.start', async ($, e, next) => {
    await $.command.register({
      name: 'gtme',
      description: 'Open the gtme console beside the chat',
      // Opens while a run is in flight, which is when it is wanted.
      immediate: true,
    })
    void refresh($)

    return next(e)
  })

  on('command.run', { command: 'gtme' }, async $ => {
    await refresh($)
    await $.ui.open({ id: PANE, title: 'gtme', columns: DOCK_COLUMNS })

    return { text: 'gtme console opened.' }
  })

  on('tool.call', { tool: 'Bash' }, async ($, e, next) => {
    if (!GTME_VERB.test(e.command)) {
      return next(e)
    }

    // Follow the ledger and folder this command uses. Only ever a ledger that
    // exists: `gtme query` on a missing path makes one.
    const { env, unresolved } = envOf(e.command)
    const workdir = workdirOf(e.command)
    const ledger = env.GTME_LEDGER ?? null
    if (unresolved.length > 0 || (ledger !== null && !(await $.fs.exists(ledger)))) {
      $.ui.toast(`gtme console: can't follow ${unresolved.join(', ') || 'GTME_LEDGER'} in that command; showing the last ledger`)
    } else {
      await update($, snap, s => ({ ...s, ledger, env, workdir }))
    }

    const mode = modeOf(e.command)
    const pipeline = pipelineOf(e.command)
    const startedAt = Date.now()
    await update($, active, () => ({ pipeline, mode, startedAt }))
    void $.ui.open({ id: PANE, title: 'gtme', columns: DOCK_COLUMNS })

    const timer = $.clock.every(1000, () => void refresh($))

    try {
      return await next(e)
    } finally {
      timer.cancel()
      await update($, active, () => null)
      await refresh($)
      const s = await read($, snap)
      const latest = s.runs[0]
      if (mode === 'simulate') {
        $.ui.toast(`${pipeline}: simulated (nothing persists)`)
      } else if (latest !== undefined && s.pending > 0) {
        $.ui.toast(`${latest.pipeline}: ${s.pending} records are waiting for you in the console`)
        void $.ui.notify(`${s.pending} records are waiting for your review`, { title: `gtme · ${latest.pipeline}` })
      } else if (latest !== undefined) {
        $.ui.toast(`${latest.pipeline}: ${latest.status}${latest.dry ? ' (dry run)' : ''} · ${spend(s.costs)}`)
      }
    }
  }).catch(($, e, next) => next(e)) // observer, not a guard: the command always goes on, never twice

  on('ui.render', { component: 'AbovePrompt' }, async ($, e, next) => {
    if (e.props.hasSurvey) {
      return next(e)
    }
    const now = await read($, active)
    const s = await read($, snap)

    if (now === null && s.pending === 0) {
      return next(e)
    }

    const { Box, Button, Text } = $.ui.resolve(e)
    const latest = s.runs[0]
    const views = latest === undefined ? [] : stepViews(s, latest.status)
    const current = views.findIndex(v => v.state === 'running' || v.state === 'waiting')
    const step = current === -1 ? undefined : views[current]

    return (
      <Box flexDirection="column">
        <Box>
          <Text color="cyan" bold>gtme </Text>
          {now !== null ? (
            <Text>
              <Text color="yellow">● </Text>
              {now.mode} {now.pipeline} · {seconds(now.startedAt)}s
              {step !== undefined && now.mode !== 'simulate' ? ` · step ${current + 1}/${views.length} ${step.id} (${step.detail})` : ''}
              {now.mode !== 'simulate' ? ` · ${spend(s.costs)}` : ''}{' '}
            </Text>
          ) : (
            <Text>
              <Text color="magenta">◆ </Text>
              {s.pending} of {s.review.length} records waiting for you in {latest?.pipeline ?? 'the last run'}{' '}
            </Text>
          )}
          <Button key="open" label="Open" onPress={() => void $.ui.open({ id: PANE, title: 'gtme', columns: DOCK_COLUMNS, focus: true })} />
        </Box>
        {await next(e)}
      </Box>
    )
  })

  on('ui.render', { component: 'Pane', requestId: PANE }, async ($, e) => {
    const { Box, Button, Input, Text } = $.ui.resolve(e)
    const s = await read($, snap)
    const now = await read($, active)
    const allDrafts = await read($, drafts)
    const nowDrafting = await read($, drafting)
    const latest = s.runs[0]
    const views = latest === undefined ? [] : stepViews(s, latest.status)
    const notes = latest === undefined ? [] : receiptNotes(s.receipt, views.map(v => v.id))
    const width = Math.max(8, ...views.map(v => v.id.length)) + 1
    const useWidth = Math.max(10, ...views.map(v => v.use.length)) + 1
    const isAllAnswered = s.review.length > 0 && s.pending === 0
    // While records wait on the person the queue is what matters: the pane
    // drops the step rows, notes and recent runs (the diagram still shows
    // each step's state), so the queue fits a pane seated above the prompt.
    // Docked beside a fullscreen transcript the pane has the full height and
    // a narrow column; seated above the prompt it has a few rows and the full
    // width. Only the inline seat needs the compact review layout.
    const isDocked = e.props.placement === 'dock'
    const isReviewing = s.review.length > 0 && !isDocked

    return (
      <Box flexDirection="column" gap={1}>
        <Box>
          <Text color="cyan" bold>gtme console </Text>
          <Text dimColor>{ledgerLabel(s.ledger)} </Text>
          <Button key="refresh" label="Refresh" onPress={() => void refresh($)} />
        </Box>

        {s.error !== null && <Text color="red">{s.error}</Text>}

        {latest === undefined ? (
          <Text dimColor>No runs in this ledger yet.</Text>
        ) : (
          <Box flexDirection="column">
            <Box>
              <Text bold>{latest.pipeline} </Text>
              <Text color={latest.status === 'running' ? 'yellow' : latest.status === 'failed' ? 'red' : latest.status === 'pending' ? 'magenta' : 'green'}>
                {latest.status}
              </Text>
              <Text dimColor>
                {latest.dry ? ' · dry run' : ' · armed'}
                {now !== null ? ` · ${seconds(now.startedAt)}s` : ''} · spend {spend(s.costs)}
              </Text>
            </Box>
            <Text>
              {views.map((v, i) => (
                <Text>
                  <Text color={COLOR[v.state]} bold={v.state === 'running' || v.state === 'pending'}>
                    {v.id}
                  </Text>
                  {i < views.length - 1 ? <Text dimColor> ─▶ </Text> : ''}
                </Text>
              ))}
            </Text>
            {!isReviewing && <Box flexDirection="column" marginTop={1}>
              {views.map(v => (
                <Text mono>
                  <Text color={COLOR[v.state]}>{GLYPH[v.state]} </Text>
                  <Text bold={v.state === 'running' || v.state === 'pending'}>{v.id.padEnd(width)}</Text>
                  <Text dimColor>{v.use.padEnd(useWidth)}</Text>
                  <Text color={v.state === 'running' ? 'yellow' : v.state === 'pending' ? 'magenta' : undefined} dimColor={v.state === 'done' || v.state === 'waiting'}>
                    {v.detail}
                  </Text>
                </Text>
              ))}
            </Box>}
          </Box>
        )}

        {latest !== undefined && s.review.length > 0 && (
          <Box flexDirection="column">
            <Text color="magenta" bold>
              ◆ waiting for you · {s.pending} of {s.review.length} unanswered
            </Text>
            {s.review.map(item => {
              const controls = controlsOf(item)
              const choices = controls.filter(c => c.options.length > 0)
              const texts = controls.filter(c => c.options.length === 0)
              const id = `${item.step}/${item.key}`
              const isWriting = item.answer === null && texts.length > 0
              const isStacked = isDocked || isWriting || item.referent !== null
              const mark = item.answer === null ? null : item.answer.startsWith('rejected') ? '✗' : '✓'
              const head = (
                <Text wrap={isStacked ? 'wrap' : 'truncate-end'}>
                  {mark !== null && <Text color={mark === '✗' ? 'red' : 'green'}>{mark} </Text>}
                  <Text bold={item.answer === null} dimColor={item.answer !== null}>{item.key}</Text>
                  <Text dimColor>{isStacked ? '\n' : '  '}{item.answer ?? item.surface}</Text>
                </Text>
              )
              const ask = <Button key={`ask:${id}`} label="Ask" onPress={() => void $.prompt.submit({ text: askText(item, latest) })} />

              return (
                <Box flexDirection={isStacked ? 'column' : 'row'} gap={isStacked ? 0 : 1} marginTop={isStacked ? 1 : 0}>
                  {isStacked && head}
                  {isStacked && item.answer === null && item.referent !== null && texts.length === 0 && (
                    <Text italic>“{item.referent}”</Text>
                  )}
                  {item.answer === null && item.role === 'filter' && (
                    <Box gap={1}>
                      <Button key={`approve:${item.key}`} label="Approve" variant="primary" onPress={() => void answer($, latest, item, true)} />
                      <Button key={`reject:${item.key}`} label="Reject" onPress={() => void answer($, latest, item, false)} />
                      {ask}
                    </Box>
                  )}
                  {item.answer === null && choices.map(c => (
                    <Box gap={1}>
                      {c.options.map((option, i) => (
                        <Button
                          key={`choose:${id}:${c.field}:${option}`}
                          label={option}
                          variant={i === 0 ? 'primary' : undefined}
                          onPress={() => void answerWith($, latest, item, { [c.field]: option })}
                        />
                      ))}
                      {texts.length === 0 && ask}
                    </Box>
                  ))}
                  {isWriting && texts.map(c => (
                    <Input
                      key={`write:${id}:${c.field}`}
                      label={texts.length > 1 ? c.field.split('.').at(-1) : undefined}
                      placeholder="Write it, or press Draft with Claude"
                      value={draftOf(allDrafts, item, c.field)}
                      rows={3}
                      submitLabel="save"
                      onInput={value => void update($, drafts, d => ({ ...d, [id]: { ...(d[id] ?? {}), [c.field]: value } }))}
                      onSubmit={value => void saveText($, latest, item, { field: c.field, value })}
                    />
                  ))}
                  {isWriting && (
                    <Box gap={1}>
                      <Button key={`save:${id}`} label="Save" variant="primary" onPress={() => void saveText($, latest, item)} />
                      <Button
                        key={`draft:${id}`}
                        label={nowDrafting === id ? 'Drafting…' : 'Draft with Claude'}
                        onPress={() => {
                          const first = texts[0]
                          if (first !== undefined && nowDrafting === null) {
                            void draftWithClaude($, item, first.field)
                          }
                        }}
                      />
                      {ask}
                      {allDrafts[id]?.__ai !== undefined && <Text dimColor>drafted by Claude</Text>}
                    </Box>
                  )}
                  {!isStacked && head}
                </Box>
              )
            })}
            {isAllAnswered && (
              <Box marginTop={1} gap={1}>
                <Text color="green">Every record is answered. </Text>
                <Button
                  key="collect"
                  label="Collect: have Claude run it"
                  variant="primary"
                  onPress={() =>
                    void $.prompt.submit({
                      text: `Every record waiting in run ${latest.id} (pipeline ${latest.pipeline}) is answered in the gtme console. Collect them: run \`gtme run\` on that pipeline again, from ${s.workdir ?? 'its folder'} with the same environment as the last run, then tell me in two sentences what was approved and delivered.`,
                    })
                  }
                />
              </Box>
            )}
          </Box>
        )}

        {!isReviewing && notes.length > 0 && (
          <Box flexDirection="column">
            {notes.map(line => (
              <Text dimColor={!line.startsWith('total:')}>{line}</Text>
            ))}
          </Box>
        )}

        {!isReviewing && s.runs.length > 1 && (
          <Box flexDirection="column">
            <Text dimColor>recent runs</Text>
            {s.runs.slice(1).map(run => (
              <Text mono dimColor>
                {run.started_at.slice(0, 16).replace('T', ' ')}  {run.pipeline.padEnd(14)} {run.status}
                {run.dry ? ' (dry)' : ''}
              </Text>
            ))}
          </Box>
        )}
      </Box>
    )
  })
}
