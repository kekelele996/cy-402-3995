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
// 对方当事人字段使用指针：不传表示不修改，传空串表示清空（证件号清空后冲突预检将只提示不阻断）。
type CaseUpdateRequest struct {
	Title            *string  `json:"title" binding:"omitempty,max=200"`
	Summary          *string  `json:"summary"`
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

// ConflictCase 一条冲突记录：某律师在某未结案件中代理了证件号相同的当事人。
type ConflictCase struct {
	LawyerID         uint64 `json:"lawyer_id"`
	LawyerName       string `json:"lawyer_name"`
	CaseID           uint64 `json:"case_id"`
	CaseNo           string `json:"case_no"`
	CaseTitle        string `json:"case_title"`
	OpponentName     string `json:"opponent_name"`
	OpponentIDNumber string `json:"opponent_id_number"`
}

// ConflictCheck 律师利益冲突预检结果。
type ConflictCheck struct {
	HasConflict bool           `json:"has_conflict"`
	Reason      string         `json:"reason"`
	Cases       []ConflictCase `json:"cases"`
	Checked     bool           `json:"checked"` // 是否实际执行了预检；对方证件号缺失时为 false
	Warning     string         `json:"warning"`
}

// CaseResult 案件写操作/详情响应：案件本体 + 冲突预检结果。
type CaseResult struct {
	Case      any            `json:"case"`
	Conflicts *ConflictCheck `json:"conflicts"`
	Warning   string         `json:"warning,omitempty"`
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
