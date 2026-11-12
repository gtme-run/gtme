export type RunRow = {
  id: string
  pipeline: string
  status: string
  dry: number
  started_at: string
}

export type StepCount = { step_id: string; event: string; n: number }

export type PlanStep = { id: string; use: string }

export type CostRow = { basis: string; usd: number }

// One record waiting on a person, from `gtme show --run RUN --pending`,
// with the answer the ledger already holds for it (an `answered` event).
export type ReviewItem = {
  key: string
  step: string
  role: string
  // What the step shows the person, the referent's line taken out.
  surface: string
  // The value a review grades or a compose `of:` revises, when the step has one.
  referent: string | null
  // What the step accepts, as `gtme show --pending` spells it:
  // `copy.grade=ship|rewrite|drop`, `copy.first_line=<text>`.
  outputs: string[]
  answer: string | null
}

// Text a person is writing into a compose step's fields, by `step/key`, then
// field. `__ai` holds the text Claude drafted, so the answer's note can say so.
export type Drafts = Record<string, Record<string, string>>

export type Snapshot = {
  ledger: string | null
  // HOME / GTME_HOME / GTME_LEDGER the last gtme command set, passed on to every call
  env: Record<string, string>
  workdir: string | null
  runs: RunRow[]
  plan: PlanStep[]
  steps: StepCount[]
  costs: CostRow[]
  receipt: string
  review: ReviewItem[]
  pending: number
  error: string | null
  refreshedAt: number
}

export type Active = {
  pipeline: string
  mode: 'armed' | 'dry-run' | 'simulate' | 'answer'
  startedAt: number
}

declare module 'claude-code' {
  interface PluginState {
    gtme: {
      snap: Shaped<Snapshot>
      active: Active | null
      drafts: Drafts
      drafting: string | null
    }
  }
}
