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
  surface: string
  answer: string | null
}

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
    }
  }
}
