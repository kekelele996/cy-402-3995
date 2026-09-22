package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"cylawcase/internal/constants"
	"cylawcase/internal/model"
	"cylawcase/internal/repository"
	"cylawcase/internal/util"
)

// LawyerConflict 单个律师的利益冲突明细（service 层）。
type LawyerConflict struct {
	LawyerID         uint64
	LawyerName       string
	ClientName       string
	OpponentName     string
	OpponentIDNumber string
	ConflictCaseNos  []string
}

// LawyerConflictError 利益冲突业务错误，携带结构化冲突明细供 handler 透出。
type LawyerConflictError struct {
	Conflicts []LawyerConflict
	Message   string
}

// Error 实现 error 接口。
func (e *LawyerConflictError) Error() string { return e.Message }

// CaseService 案件业务逻辑。
type CaseService struct {
	repo       *repository.CaseRepository
	clientRepo *repository.ClientRepository
	userRepo   *repository.UserRepository
	logger     *slog.Logger
}

// NewCaseService 构造案件服务。
func NewCaseService(repo *repository.CaseRepository, clientRepo *repository.ClientRepository,
	userRepo *repository.UserRepository, logger *slog.Logger) *CaseService {
	return &CaseService{repo: repo, clientRepo: clientRepo, userRepo: userRepo, logger: logger}
}

// Create 创建案件，同时记录对方当事人姓名与证件号（资料缺失只提示不阻断）。
func (s *CaseService) Create(clientID, leadLawyerID uint64, title, caseType, summary, opponentName, opponentIDNumber string,
	acceptDate *time.Time, coLawyerIDs []uint64) (*model.Case, error) {
	if !constants.IsValidCaseType(caseType) {
		return nil, util.NewAppError(constants.CodeValidationFailed, "Case[case_type="+caseType+"] create: invalid type")
	}
	if _, err := s.clientRepo.FindByID(clientID); err != nil {
		return nil, util.Wrap(err, "Case[client_id=%d] create: client not found", clientID)
	}
	if _, err := s.userRepo.FindByID(leadLawyerID); err != nil {
		return nil, util.Wrap(err, "Case[lead_lawyer_id=%d] create: lawyer not found", leadLawyerID)
	}
	co := jsonCoLawyers(coLawyerIDs)
	c := &model.Case{
		CaseNo:           genCaseNo(),
		Title:            title,
		CaseType:         caseType,
		Status:           constants.CaseStatusFiled,
		AcceptDate:       acceptDate,
		Summary:          summary,
		ClientID:         clientID,
		OpponentName:     strings.TrimSpace(opponentName),
		OpponentIDNumber: strings.TrimSpace(opponentIDNumber),
		LeadLawyerID:     leadLawyerID,
		CoLawyerIDs:      co,
	}
	if err := s.repo.Create(c); err != nil {
		s.logger.Error(constants.LogCaseCreateFailed, "error", err.Error())
		return nil, util.Wrap(err, "Case[title=%s] create failed", title)
	}
	s.logger.Info(constants.LogCaseCreateSuccess, "case_id", c.ID, "case_no", c.CaseNo,
		"opponent_name", c.OpponentName, "opponent_id_number", c.OpponentIDNumber)
	return c, nil
}

// Update 更新案件信息，可补录/修改/清空对方当事人姓名与证件号。
// 传 nil 表示该字段不变，传空串表示清空（清空后再次分配将仅提示不阻断预检）。
func (s *CaseService) Update(id uint64, title, summary string, opponentName, opponentIDNumber *string, coLawyerIDs []uint64) (*model.Case, error) {
	c, err := s.repo.FindByID(id)
	if err != nil {
		return nil, util.Wrap(err, "Case[id=%d] update find failed", id)
	}
	if title != "" {
		c.Title = title
	}
	if summary != "" {
		c.Summary = summary
	}
	if opponentName != nil {
		c.OpponentName = strings.TrimSpace(*opponentName)
	}
	if opponentIDNumber != nil {
		c.OpponentIDNumber = strings.TrimSpace(*opponentIDNumber)
	}
	if coLawyerIDs != nil {
		c.CoLawyerIDs = jsonCoLawyers(coLawyerIDs)
	}
	if err := s.repo.Update(c); err != nil {
		return nil, util.Wrap(err, "Case[id=%d] update save failed", id)
	}
	s.logger.Info(constants.LogCaseUpdateSuccess, "case_id", c.ID,
		"opponent_name", c.OpponentName, "opponent_id_number", c.OpponentIDNumber)
	return c, nil
}

// ChangeStatus 案件状态流转。
func (s *CaseService) ChangeStatus(id uint64, operatorRole string, status string) (*model.Case, error) {
	c, err := s.repo.FindByID(id)
	if err != nil {
		return nil, util.Wrap(err, "Case[id=%d] status change find failed", id)
	}
	if !constants.IsValidCaseStatus(status) {
		return nil, util.NewAppError(constants.CodeValidationFailed, "Case[id="+u64(id)+"] status invalid: "+status)
	}
	if operatorRole != constants.RoleAdmin && !canFlow(c.Status, status) {
		return nil, util.NewAppError(constants.CodeCaseStatusConflict, "Case[id="+u64(id)+"] status conflict: "+c.Status+" -> "+status)
	}
	c.Status = status
	if status == constants.CaseStatusClosed && c.CloseDate == nil {
		now := time.Now()
		c.CloseDate = &now
	}
	if err := s.repo.Update(c); err != nil {
		s.logger.Error(constants.LogCaseStatusChangeFailed, "error", err.Error())
		return nil, util.Wrap(err, "Case[id=%d] status change save failed", id)
	}
	s.logger.Info(constants.LogCaseStatusChangeSuccess, "case_id", c.ID, "status", status)
	return c, nil
}

// Assign 分配主办及协作律师。
// 利益冲突预检（任何角色均不可绕过，包括管理员）：任一待分配律师正在其他未结案件中
// 代理证件号与本案对方当事人相同的当事人时，整次拒绝并列出冲突案号，原分配保持不变。
// 对方资料缺失只提示不阻断。
func (s *CaseService) Assign(id, leadLawyerID uint64, coLawyerIDs []uint64) (*model.Case, string, error) {
	c, err := s.repo.FindByID(id)
	if err != nil {
		return nil, "", util.Wrap(err, "Case[id=%d] assign find failed", id)
	}

	lawyerIDs := dedupeLawyerIDs(leadLawyerID, coLawyerIDs)
	for _, lid := range lawyerIDs {
		if _, err := s.userRepo.FindByID(lid); err != nil {
			return nil, "", util.Wrap(err, "Case[id=%d] assign failed: lawyer not match", id)
		}
	}

	// 资料缺失：跳过预检，仅返回提示，不阻断分配。
	if strings.TrimSpace(c.OpponentName) == "" || strings.TrimSpace(c.OpponentIDNumber) == "" {
		warning := constants.MsgOpponentInfoMissing
		s.logger.Warn(constants.LogCaseAssignCheckSkipped, "case_id", id, "case_no", c.CaseNo,
			"opponent_name", c.OpponentName, "opponent_id_number", c.OpponentIDNumber)
		c.LeadLawyerID = leadLawyerID
		if coLawyerIDs != nil {
			c.CoLawyerIDs = jsonCoLawyers(coLawyerIDs)
		}
		if err := s.repo.Update(c); err != nil {
			s.logger.Error(constants.LogCaseAssignFailed, "error", err.Error())
			return nil, "", util.Wrap(err, "Case[id=%d] assign save failed", id)
		}
		s.logger.Info(constants.LogCaseAssignSuccess, "case_id", c.ID, "lead_lawyer_id", leadLawyerID)
		return c, warning, nil
	}

	conflicts, err := s.checkConflicts(c, lawyerIDs)
	if err != nil {
		return nil, "", err
	}
	if len(conflicts) > 0 {
		s.logger.Warn(constants.LogCaseAssignConflict, "case_id", id, "case_no", c.CaseNo,
			"opponent_id_number", c.OpponentIDNumber, "conflicts", len(conflicts))
		return nil, "", &LawyerConflictError{
			Conflicts: conflicts,
			Message:   buildConflictMessage(id, c, conflicts),
		}
	}

	c.LeadLawyerID = leadLawyerID
	if coLawyerIDs != nil {
		c.CoLawyerIDs = jsonCoLawyers(coLawyerIDs)
	}
	if err := s.repo.Update(c); err != nil {
		s.logger.Error(constants.LogCaseAssignFailed, "error", err.Error())
		return nil, "", util.Wrap(err, "Case[id=%d] assign save failed", id)
	}
	s.logger.Info(constants.LogCaseAssignSuccess, "case_id", c.ID, "lead_lawyer_id", leadLawyerID)
	return c, "", nil
}

// checkConflicts 针对所有待分配律师执行利益冲突预检。
func (s *CaseService) checkConflicts(c *model.Case, lawyerIDs []uint64) ([]LawyerConflict, error) {
	candidates, err := s.repo.ListOpenByClientIDNumber(strings.TrimSpace(c.OpponentIDNumber), c.ID)
	if err != nil {
		return nil, util.Wrap(err, "Case[id=%d] assign conflict check failed", c.ID)
	}
	if len(candidates) == 0 {
		return nil, nil
	}

	var conflicts []LawyerConflict
	for _, lid := range lawyerIDs {
		var hitCaseNos []string
		for _, other := range candidates {
			if other.LeadLawyerID == lid || containsLawyerID(parseCoLawyerIDs(other.CoLawyerIDs), lid) {
				hitCaseNos = append(hitCaseNos, other.CaseNo)
			}
		}
		if len(hitCaseNos) == 0 {
			continue
		}
		lawyerName := fmt.Sprintf("#%d", lid)
		if u, err := s.userRepo.FindByID(lid); err == nil {
			if u.RealName != "" {
				lawyerName = u.RealName
			} else {
				lawyerName = u.Username
			}
		}
		clientName := ""
		if cl, err := s.clientRepo.FindByID(candidates[0].ClientID); err == nil {
			clientName = cl.Name
		}
		conflicts = append(conflicts, LawyerConflict{
			LawyerID:         lid,
			LawyerName:       lawyerName,
			ClientName:       clientName,
			OpponentName:     c.OpponentName,
			OpponentIDNumber: c.OpponentIDNumber,
			ConflictCaseNos:  hitCaseNos,
		})
	}
	return conflicts, nil
}

// List 分页查询案件。
func (s *CaseService) List(page, pageSize int, caseType, status string, lawyerID uint64, startDate, endDate *time.Time) ([]model.Case, int64, error) {
	return s.repo.List(page, pageSize, caseType, status, lawyerID, startDate, endDate)
}

// Get 案件详情。
func (s *CaseService) Get(id uint64) (*model.Case, error) {
	return s.repo.FindByID(id)
}

// IsLawyerConflictError 判断是否为利益冲突错误。
func IsLawyerConflictError(err error) (*LawyerConflictError, bool) {
	var ce *LawyerConflictError
	if errors.As(err, &ce) {
		return ce, true
	}
	return nil, false
}

// buildConflictMessage 拼接包含实体名、字段名、角色名与冲突案号的错误文案。
func buildConflictMessage(id uint64, c *model.Case, conflicts []LawyerConflict) string {
	var b strings.Builder
	b.WriteString("Case[id=" + u64(id) + "] assign rejected by lawyer[role=lawyer] conflict-of-interest precheck: ")
	b.WriteString("opponent[")
	b.WriteString(c.OpponentName)
	b.WriteString(",id_number=")
	b.WriteString(c.OpponentIDNumber)
	b.WriteString("] is already represented as client in open cases; ")
	for i, cf := range conflicts {
		if i > 0 {
			b.WriteString("; ")
		}
		b.WriteString("lawyer=")
		b.WriteString(cf.LawyerName)
		b.WriteString(" conflict_cases=")
		b.WriteString(strings.Join(cf.ConflictCaseNos, ","))
	}
	return b.String()
}

// canFlow 案件状态机：filed->investigating->hearing->closed->archived，允许回退到上一步。
func canFlow(from, to string) bool {
	idx := map[string]int{constants.CaseStatusFiled: 0, constants.CaseStatusInvestigating: 1,
		constants.CaseStatusHearing: 2, constants.CaseStatusClosed: 3, constants.CaseStatusArchived: 4}
	a, okA := idx[from]
	b, okB := idx[to]
	if !okA || !okB {
		return false
	}
	return b == a+1 || b == a-1 || b == a
}

func jsonCoLawyers(ids []uint64) model.CoLawyerJSON {
	if ids == nil {
		ids = []uint64{}
	}
	raw, _ := json.Marshal(ids)
	return model.CoLawyerJSON(raw)
}

// parseCoLawyerIDs 解析协作律师 ID JSON，异常或空值返回空切片。
func parseCoLawyerIDs(raw model.CoLawyerJSON) []uint64 {
	if len(raw) == 0 {
		return []uint64{}
	}
	var ids []uint64
	if err := json.Unmarshal(raw, &ids); err != nil {
		return []uint64{}
	}
	return ids
}

// dedupeLawyerIDs 合并主办与协作律师并去重。
func dedupeLawyerIDs(lead uint64, co []uint64) []uint64 {
	seen := map[uint64]struct{}{lead: {}}
	out := []uint64{lead}
	for _, id := range co {
		if id == 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func containsLawyerID(list []uint64, target uint64) bool {
	for _, v := range list {
		if v == target {
			return true
		}
	}
	return false
}

func genCaseNo() string {
	return fmt.Sprintf("CY%d%04d", time.Now().Year(), time.Now().UnixNano()%10000)
}

func u64(v uint64) string {
	return fmt.Sprintf("%d", v)
}
