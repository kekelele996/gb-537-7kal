export interface ApiEnvelope<T> {
  code: string
  message: string
  data: T
  details?: unknown
  request_id: string
}

export interface Paginated<T> {
  items: T[]
  total: number
  page: number
  size: number
}

export type LoadState = 'idle' | 'loading' | 'ready' | 'error'

