import type { ScenarioState } from './enums/scenario-state'
import type { AffectedService } from './rollover-scenario'

export interface RolloverSignoff {
  service_id: number
  service_code: string
  criticality: string
  note: string
  signed_by: number
  signed_by_name: string
  locked_at?: string
  updated_at: string
}

export interface RolloverGateTodo {
  kind: 'signoff' | 'replay'
  service_id?: number
  service_code?: string
  message: string
}

export interface RolloverReviewGate {
  scenario_id: number
  scenario_state: ScenarioState
  required: AffectedService[]
  signoffs: RolloverSignoff[]
  pending: RolloverGateTodo[]
  replay_verified: boolean
  satisfied: boolean
}
