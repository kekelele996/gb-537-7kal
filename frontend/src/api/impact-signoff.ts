import type { CreateImpactSignoffInput, SignoffGateStatus } from '../types/impact-signoff'
import { apiRequest } from './client'

export const impactSignoffApi = {
  gate: (scenarioId: number) => apiRequest<SignoffGateStatus>(`/rollover-scenarios/${scenarioId}/signoffs`),
  register: (scenarioId: number, input: CreateImpactSignoffInput) => apiRequest<SignoffGateStatus>(`/rollover-scenarios/${scenarioId}/signoffs`, { method: 'POST', body: JSON.stringify(input) }),
}
