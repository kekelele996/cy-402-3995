import request from '@/utils/request'
import type { CaseResult } from '@/types'

export interface CaseCreatePayload {
  client_id: number
  lead_lawyer_id: number
  title: string
  case_type: string
  summary?: string
  accept_date?: string
  co_lawyer_ids?: number[]
  opponent_name?: string
  opponent_id_number?: string
}

export interface CaseUpdatePayload {
  title?: string
  summary?: string
  co_lawyer_ids?: number[]
  opponent_name?: string
  opponent_id_number?: string
}

export function listCases(params: Record<string, unknown>) {
  return request.get('/cases', { params })
}

export function getCase(id: number) {
  return request.get(`/cases/${id}`)
}

export function createCase(data: CaseCreatePayload) {
  return request.post<unknown, { data: CaseResult }>('/cases', data)
}

export function updateCase(id: number, data: CaseUpdatePayload) {
  return request.put<unknown, { data: CaseResult }>(`/cases/${id}`, data)
}

export function changeCaseStatus(id: number, status: string) {
  return request.post(`/cases/${id}/status`, { status })
}

export function assignLawyer(id: number, data: { lead_lawyer_id: number; co_lawyer_ids?: number[] }) {
  return request.post<unknown, { data: CaseResult }>(`/cases/${id}/assign`, data)
}
