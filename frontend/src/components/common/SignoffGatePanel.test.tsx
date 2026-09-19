import { render, screen } from '@testing-library/react'
import { SignoffGatePanel } from './SignoffGatePanel'
import { useAuthStore } from '../../stores/auth'
import { useImpactSignoffStore } from '../../stores/impact-signoff'
import type { Actor } from '../../types/auth'
import type { SignoffGateStatus } from '../../types/impact-signoff'
import type { RolloverScenario } from '../../types/rollover-scenario'

const scenario = { id: 5, created_by: 7, scenario_state: 'executing', updated_at: '2032-04-02T08:00:00Z' } as RolloverScenario
const reviewer: Actor = { user_id: 9, username: 'reviewer', display_name: 'Independent Reviewer', team: 'Security Assurance', role: 'security_reviewer' }
const creator: Actor = { user_id: 7, username: 'operator', display_name: 'Operator', team: 'PKI Platform', role: 'pki_operator' }

function seedGate(gate: SignoffGateStatus, user: Actor) {
  useAuthStore.setState({ user, token: 'token' })
  useImpactSignoffStore.setState({ gate, status: 'ready', error: '', fetchGate: async () => undefined })
}

const pendingGate: SignoffGateStatus = {
  scenario_id: 5,
  scenario_state: 'executing',
  required_services: [{ service_id: 11, service_code: 'PAYMENTS-API', criticality: 'critical' }],
  signoffs: [],
  todos: [
    { type: 'signoff_missing', service_id: 11, service_code: 'PAYMENTS-API', message: 'critical affected service PAYMENTS-API is missing a disposition sign-off' },
    { type: 'replay_required', message: 'historical result replay has not passed for the frozen evidence' },
  ],
  replay_verified: false,
  gate_satisfied: false,
}

describe('SignoffGatePanel', () => {
  it('lists pending todos and lets an independent reviewer register a disposition', () => {
    seedGate(pendingGate, reviewer)
    render(<SignoffGatePanel scenario={scenario} />)
    expect(screen.getByText('关键影响签收闸门')).toBeInTheDocument()
    expect(screen.getByText('待办 2 项')).toBeInTheDocument()
    expect(screen.getByText('待签收')).toBeInTheDocument()
    expect(screen.getByText(/PAYMENTS-API is missing a disposition sign-off/)).toBeInTheDocument()
    expect(screen.getByText(/historical result replay has not passed/)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '登记签收' })).toBeInTheDocument()
  })

  it('hides the registration form from the scenario creator', () => {
    seedGate(pendingGate, creator)
    render(<SignoffGatePanel scenario={scenario} />)
    expect(screen.queryByRole('button', { name: '登记签收' })).not.toBeInTheDocument()
    expect(screen.getByText('待签收')).toBeInTheDocument()
  })

  it('shows the latest signer and disposition once the gate is satisfied', () => {
    seedGate({
      ...pendingGate,
      signoffs: [{ id: 3, scenario_id: 5, service_id: 11, service_code: 'PAYMENTS-API', disposition: 'cutover rehearsed with payments on-call', signed_by: 9, signed_by_name: 'reviewer', created_at: '2032-04-02T09:00:00Z', updated_at: '2032-04-02T09:30:00Z' }],
      todos: [],
      replay_verified: true,
      gate_satisfied: true,
    }, reviewer)
    render(<SignoffGatePanel scenario={scenario} />)
    expect(screen.getByText('闸门已就绪')).toBeInTheDocument()
    expect(screen.getByText('cutover rehearsed with payments on-call')).toBeInTheDocument()
    expect(screen.getByText(/签收人 reviewer/)).toBeInTheDocument()
    expect(screen.getByText('已签收')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '更新签收' })).toBeInTheDocument()
  })
})
