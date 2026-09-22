import request from '@/utils/request'
import type { CaseAssignResult, CaseItem } from '@/types'

export interface CasePayload {
  title?: string
  case_type?: string
  client_id?: number
  lead_lawyer_id?: number
  accept_date?: string
  summary?: string
  co_lawyer_ids?: number[]
  opponent_name?: string
  opponent_id_number?: string
}

export interface AssignPayload {
  lead_lawyer_id: number
  co_lawyer_ids?: number[]
}

export function listCases(params: Record<string, unknown>) {
  return request.get('/cases', { params })
}

export function getCase(id: number) {
  return request.get(`/cases/${id}`)
}

export function createCase(data: CasePayload) {
  return request.post('/cases', data)
}

export function updateCase(id: number, data: Partial<CaseItem>) {
  return request.put(`/cases/${id}`, data)
}

export function changeCaseStatus(id: number, status: string) {
  return request.post(`/cases/${id}/status`, { status })
}

export function assignLawyer(id: number, data: AssignPayload) {
  return request.post(`/cases/${id}/assign`, data) as Promise<{ data: CaseAssignResult }>
}
