import { FactCheckRounded, PlaylistAddCheckRounded, ReplayRounded, TaskAltRounded } from '@mui/icons-material'
import { Alert, Box, Button, Chip, TextField, Typography } from '@mui/material'
import { FormEvent, useEffect, useState } from 'react'
import { errorMessage } from '../../api/client'
import { useAuth } from '../../hooks/useAuth'
import { useImpactSignoffStore } from '../../stores/impact-signoff'
import type { RolloverScenario } from '../../types/rollover-scenario'
import { formatDateTime } from '../../utils/date'

const OPEN_STATES = ['simulated', 'ready', 'executing']

export function SignoffGatePanel({ scenario }: { scenario: RolloverScenario }) {
  const { gate, status, error, fetchGate, register } = useImpactSignoffStore()
  const { can, user } = useAuth()
  const [drafts, setDrafts] = useState<Record<number, string>>({})
  const [feedback, setFeedback] = useState('')
  const [busyService, setBusyService] = useState<number | null>(null)

  useEffect(() => { void fetchGate(scenario.id) }, [fetchGate, scenario.id, scenario.updated_at])

  const canSign = can('scenario.verify') && scenario.created_by !== user?.user_id && OPEN_STATES.includes(scenario.scenario_state)
  const current = gate && gate.scenario_id === scenario.id ? gate : null

  const submit = async (event: FormEvent, serviceId: number) => {
    event.preventDefault()
    setFeedback('')
    setBusyService(serviceId)
    try {
      await register(scenario.id, { service_id: serviceId, disposition: (drafts[serviceId] ?? '').trim() })
      setDrafts((currentDrafts) => ({ ...currentDrafts, [serviceId]: '' }))
    } catch (cause) {
      setFeedback(errorMessage(cause))
    } finally {
      setBusyService(null)
    }
  }

  return <section className="signoff-gate">
    <Box className="detail-section-head">
      <Typography variant="h3">关键影响签收闸门</Typography>
      <span>{current ? (current.gate_satisfied ? '闸门已就绪' : `待办 ${current.todos.length} 项`) : '正在加载'}</span>
    </Box>
    {feedback && <Alert severity="error" onClose={() => setFeedback('')}>{feedback}</Alert>}
    {status === 'error' && <Alert severity="error">{error}</Alert>}
    {!current && status !== 'error' && <Typography className="signoff-empty">正在加载签收闸门…</Typography>}
    {current && <>
      <Box className={current.replay_verified ? 'signoff-row is-pass' : 'signoff-row is-todo'}>
        <ReplayRounded />
        <Box>
          <strong>历史结果回放</strong>
          <Typography>{current.replay_verified ? '重放证据与冻结历史结果一致。' : '回放未通过：请先运行“重放一致性”并确认与冻结证据一致。'}</Typography>
        </Box>
        <Chip size="small" label={current.replay_verified ? '已通过' : '待办'} color={current.replay_verified ? 'success' : 'warning'} />
      </Box>
      {!current.required_services.length && <Typography className="signoff-empty">本次仿真未列出关键受影服务，无需处置签收。</Typography>}
      {current.required_services.map((service) => {
        const signoff = current.signoffs.find((item) => item.service_id === service.service_id)
        return <Box key={service.service_id} className={signoff ? 'signoff-row is-pass' : 'signoff-row is-todo'}>
          {signoff ? <TaskAltRounded /> : <PlaylistAddCheckRounded />}
          <Box>
            <strong>{service.service_code} · 关键受影服务</strong>
            {signoff ? <>
              <Typography>{signoff.disposition}</Typography>
              <Typography className="signoff-meta">签收人 {signoff.signed_by_name} · 登记于 {formatDateTime(signoff.updated_at)}{signoff.applied_at ? ` · 已随复核生效 ${formatDateTime(signoff.applied_at)}` : ''}</Typography>
            </> : <Typography>待签收：需要安全复核员登记该服务的处置说明。</Typography>}
            {canSign && <Box component="form" className="signoff-form" onSubmit={(event) => submit(event, service.service_id)}>
              <TextField size="small" multiline minRows={2} label={signoff ? '更新处置说明（同一服务仅保留最新一次）' : '处置说明'} value={drafts[service.service_id] ?? ''} onChange={(event) => setDrafts({ ...drafts, [service.service_id]: event.target.value })} required />
              <Button type="submit" variant="outlined" disabled={busyService === service.service_id}>{busyService === service.service_id ? '正在登记…' : signoff ? '更新签收' : '登记签收'}</Button>
            </Box>}
          </Box>
          <Chip size="small" label={signoff ? '已签收' : '待签收'} color={signoff ? 'success' : 'warning'} />
        </Box>
      })}
      {!!current.todos.length && <Alert severity="warning" icon={<FactCheckRounded />} className="signoff-todos">
        复核待办：
        <ul>{current.todos.map((todo, index) => <li key={`${todo.type}-${todo.service_id ?? index}`}>{todo.message}</li>)}</ul>
      </Alert>}
    </>}
  </section>
}
