import type { ScenarioState } from './enums/scenario-state'

export interface ImpactSignoff {
  id: number
  scenario_id: number
  service_id: number
  service_code: string
  disposition: string
  signed_by: number
  signed_by_name: string
  applied_at?: string
  created_at: string
  updated_at: string
}

export interface CriticalService {
  service_id: number
  service_code: string
  criticality: string
}

export interface SignoffTodo {
  type: 'signoff_missing' | 'replay_required'
  service_id?: number
  service_code?: string
  message: string
}

export interface SignoffGateStatus {
  scenario_id: number
  scenario_state: ScenarioState
  required_services: CriticalService[]
  signoffs: ImpactSignoff[]
  todos: SignoffTodo[]
  replay_verified: boolean
  gate_satisfied: boolean
}

export interface CreateImpactSignoffInput {
  service_id: number
  disposition: string
}
