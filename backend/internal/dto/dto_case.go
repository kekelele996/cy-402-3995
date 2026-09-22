package dto

import "time"

// CaseCreateRequest 创建案件请求。
type CaseCreateRequest struct {
	ClientID         uint64   `json:"client_id" binding:"required"`
	LeadLawyerID     uint64   `json:"lead_lawyer_id" binding:"required"`
	Title            string   `json:"title" binding:"required,max=200"`
	CaseType         string   `json:"case_type" binding:"required,oneof=civil criminal administrative commercial labor"`
	Summary          string   `json:"summary"`
	AcceptDate       *string  `json:"accept_date"`
	CoLawyerIDs      []uint64 `json:"co_lawyer_ids"`
	OpponentName     string   `json:"opponent_name" binding:"max=100"`
	OpponentIDNumber string   `json:"opponent_id_number" binding:"max=50"`
}

// CaseUpdateRequest 更新案件请求。
// 对方信息字段使用指针：未传（nil）表示不变；显式传空串表示清空（资料缺失）。
type CaseUpdateRequest struct {
	Title            string   `json:"title" binding:"max=200"`
	Summary          string   `json:"summary"`
	CoLawyerIDs      []uint64 `json:"co_lawyer_ids"`
	OpponentName     *string  `json:"opponent_name" binding:"omitempty,max=100"`
	OpponentIDNumber *string  `json:"opponent_id_number" binding:"omitempty,max=50"`
}

// CaseStatusRequest 状态流转请求。
type CaseStatusRequest struct {
	Status string `json:"status" binding:"required,oneof=filed investigating hearing closed archived"`
}

// CaseAssignRequest 分配律师请求。
type CaseAssignRequest struct {
	LeadLawyerID uint64   `json:"lead_lawyer_id" binding:"required"`
	CoLawyerIDs  []uint64 `json:"co_lawyer_ids"`
}

// ParseAcceptDate 解析接受日期字符串。
func ParseAcceptDate(s string) (*time.Time, error) {
	if s == "" {
		return nil, nil
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// CaseAssignResponse 分配律师响应：案件数据 + 非阻断性提示（如对方资料缺失导致未做冲突预检）。
type CaseAssignResponse struct {
	Case    any    `json:"case"`
	Warning string `json:"warning,omitempty"`
}

// LawyerConflictInfo 单个律师的利益冲突明细。
type LawyerConflictInfo struct {
	LawyerID         uint64   `json:"lawyer_id"`
	LawyerName       string   `json:"lawyer_name"`
	ClientName       string   `json:"client_name"`
	OpponentName     string   `json:"opponent_name"`
	OpponentIDNumber string   `json:"opponent_id_number"`
	ConflictCaseNo   []string `json:"conflict_case_nos"`
}

// CaseAssignConflictData 分配被利益冲突预检拒绝时返回的结构化明细。
type CaseAssignConflictData struct {
	Reason    string               `json:"reason"`
	Conflicts []LawyerConflictInfo `json:"conflicts"`
}
