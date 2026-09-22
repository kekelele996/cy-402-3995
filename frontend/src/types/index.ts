export interface User {
  id: number
  username: string
  real_name: string
  role: string
  license_no: string
  email: string
  phone: string
  avatar: string
  created_at: string
}

export interface Client {
  id: number
  name: string
  id_number: string
  contact: string
  address: string
  remark: string
  created_at: string
}

export interface CaseItem {
  id: number
  case_no: string
  title: string
  case_type: string
  status: string
  accept_date: string | null
  close_date: string | null
  summary: string
  client_id: number
  lead_lawyer_id: number
  co_lawyer_ids: number[]
  opponent_name: string
  opponent_id_number: string
  created_at: string
}

// 一条律师利益冲突记录。
export interface ConflictCase {
  lawyer_id: number
  lawyer_name: string
  case_id: number
  case_no: string
  case_title: string
  opponent_name: string
  opponent_id_number: string
}

// 利益冲突预检结果。
export interface ConflictCheck {
  has_conflict: boolean
  reason: string
  cases: ConflictCase[]
  checked: boolean
  warning: string
}

// 案件写操作/详情响应：案件本体 + 冲突预检。
export interface CaseResult {
  case: CaseItem
  conflicts: ConflictCheck | null
  warning?: string
}

export interface DocumentItem {
  id: number
  title: string
  file_type: string
  file_url: string
  upload_time: string
  case_id: number
  uploader_id: number
  created_at: string
}

export interface Billing {
  id: number
  bill_no: string
  billing_type: string
  amount: number | string
  status: string
  case_id: number
  client_id: number
  invoice_info: string
  created_at: string
}

export interface AuditLog {
  id: number
  operator_id: number
  operator_name: string
  action: string
  entity_type: string
  entity_id: string
  detail: string
  ip: string
  created_at: string
}

export interface PageResult<T> {
  list: T[]
  total: number
  page: number
  page_size: number
}
