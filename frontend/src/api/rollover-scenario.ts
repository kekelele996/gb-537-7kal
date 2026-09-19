import type { Paginated } from '../types/common'
import type { ScenarioState } from '../types/enums/scenario-state'
import type { CreateRolloverScenarioInput, RolloverScenario } from '../types/rollover-scenario'
import type { RolloverReviewGate } from '../types/rollover-signoff'
import { apiRequest } from './client'

export const rolloverScenarioApi = {
  list: (query = '') => apiRequest<Paginated<RolloverScenario>>(`/rollover-scenarios${query}`),
  get: (id: number) => apiRequest<RolloverScenario>(`/rollover-scenarios/${id}`),
  create: (input: CreateRolloverScenarioInput) => apiRequest<RolloverScenario>('/rollover-scenarios', { method: 'POST', body: JSON.stringify(input) }),
  simulate: (id: number, idempotencyKey: string) => apiRequest<RolloverScenario>(`/rollover-scenarios/${id}/simulate`, { method: 'POST', headers: { 'Idempotency-Key': idempotencyKey } }),
  transition: (id: number, toState: ScenarioState, comment = '') => apiRequest<RolloverScenario>(`/rollover-scenarios/${id}/transition`, { method: 'POST', body: JSON.stringify({ to_state: toState, comment }) }),
  replay: (id: number) => apiRequest<RolloverScenario>(`/rollover-scenarios/${id}/replay`, { method: 'POST' }),
  compare: (id: number, otherId: number) => apiRequest<Record<string, unknown>>(`/rollover-scenarios/${id}/compare/${otherId}`),
  reviewGate: (id: number) => apiRequest<RolloverReviewGate>(`/rollover-scenarios/${id}/signoffs`),
  registerSignoff: (id: number, serviceId: number, note: string) => apiRequest<RolloverReviewGate>(`/rollover-scenarios/${id}/signoffs/${serviceId}`, { method: 'PUT', body: JSON.stringify({ note }) }),
}

