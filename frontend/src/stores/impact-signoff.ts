import { create } from 'zustand'
import { impactSignoffApi } from '../api/impact-signoff'
import { errorMessage } from '../api/client'
import type { LoadState } from '../types/common'
import type { CreateImpactSignoffInput, SignoffGateStatus } from '../types/impact-signoff'

interface ImpactSignoffState {
  gate: SignoffGateStatus | null
  status: LoadState
  error: string
  fetchGate: (scenarioId: number) => Promise<void>
  register: (scenarioId: number, input: CreateImpactSignoffInput) => Promise<SignoffGateStatus>
  reset: () => void
}

export const useImpactSignoffStore = create<ImpactSignoffState>((set) => ({
  gate: null,
  status: 'idle',
  error: '',
  fetchGate: async (scenarioId) => {
    set({ status: 'loading', error: '' })
    try {
      const gate = await impactSignoffApi.gate(scenarioId)
      set({ gate, status: 'ready' })
    } catch (cause) {
      set({ status: 'error', error: errorMessage(cause) })
    }
  },
  register: async (scenarioId, input) => {
    const gate = await impactSignoffApi.register(scenarioId, input)
    set({ gate, status: 'ready', error: '' })
    return gate
  },
  reset: () => set({ gate: null, status: 'idle', error: '' }),
}))
