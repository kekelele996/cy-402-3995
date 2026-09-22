package service

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"cylawcase/internal/constants"
	"cylawcase/internal/dto"
	"cylawcase/internal/model"
	"cylawcase/internal/repository"
	"cylawcase/internal/util"
)

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

// Create 创建案件，并在落库前对主办及协作律师执行利益冲突预检。
// 对方证件号缺失时只返回 warning 不阻断；命中冲突时整次拒绝（案件不创建），管理员也不能绕过。
func (s *CaseService) Create(clientID, leadLawyerID uint64, title, caseType, summary string,
	acceptDate *time.Time, coLawyerIDs []uint64, opponentName, opponentIDNumber string) (*dto.CaseResult, error) {
	if !constants.IsValidCaseType(caseType) {
		return nil, util.NewAppError(constants.CodeValidationFailed, "Case[case_type="+caseType+"] create: invalid type")
	}
	if _, err := s.clientRepo.FindByID(clientID); err != nil {
		return nil, util.Wrap(err, "Case[client_id=%d] create: client not found", clientID)
	}
	if _, err := s.userRepo.FindByID(leadLawyerID); err != nil {
		return nil, util.Wrap(err, "Case[lead_lawyer_id=%d] create: lawyer not found", leadLawyerID)
	}
	opponentName = strings.TrimSpace(opponentName)
	opponentIDNumber = strings.TrimSpace(opponentIDNumber)
	lawyerIDs := dedupLawyerIDs(leadLawyerID, coLawyerIDs)
	check := s.buildConflictCheck(0, lawyerIDs, opponentName, opponentIDNumber)
	if check.HasConflict {
		s.logger.Warn(constants.LogCaseConflictBlocked, "stage", "create", "title", title,
			"opponent_id_number", opponentIDNumber, "reason", check.Reason, "conflict_cases", len(check.Cases))
		return nil, util.NewAppErrorWithDetail(constants.CodeLawyerConflict,
			"Case[title="+title+"] create rejected: lawyer conflict of interest: "+check.Reason, check)
	}
	if check.Warning != "" {
		s.logger.Info(constants.LogCaseConflictCheckSkip, "stage", "create", "title", title)
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
		LeadLawyerID:     leadLawyerID,
		CoLawyerIDs:      co,
		OpponentName:     opponentName,
		OpponentIDNumber: opponentIDNumber,
	}
	if err := s.repo.Create(c); err != nil {
		s.logger.Error(constants.LogCaseCreateFailed, "error", err.Error())
		return nil, util.Wrap(err, "Case[title=%s] create failed", title)
	}
	s.logger.Info(constants.LogCaseCreateSuccess, "case_id", c.ID, "case_no", c.CaseNo,
		"opponent_name", opponentName, "opponent_id_number", opponentIDNumber)
	return &dto.CaseResult{Case: c, Conflicts: check, Warning: check.Warning}, nil
}

// Update 更新案件信息。
// 当调整协作律师或对方证件号时，必须重新通过利益冲突预检：命中冲突则整次拒绝，
// 标题、摘要等其他字段也一并保持不变。对方字段为指针：nil 不修改，空串表示清空。
func (s *CaseService) Update(id uint64, title, summary *string, coLawyerIDs []uint64,
	opponentName, opponentIDNumber *string) (*dto.CaseResult, error) {
	c, err := s.repo.FindByID(id)
	if err != nil {
		return nil, util.Wrap(err, "Case[id=%d] update find failed", id)
	}
	newOpponentName := c.OpponentName
	newOpponentIDNumber := c.OpponentIDNumber
	if opponentName != nil {
		newOpponentName = strings.TrimSpace(*opponentName)
	}
	if opponentIDNumber != nil {
		newOpponentIDNumber = strings.TrimSpace(*opponentIDNumber)
	}
	reassignCo := coLawyerIDs != nil
	opponentChanged := opponentName != nil || opponentIDNumber != nil
	needCheck := reassignCo || (opponentIDNumber != nil && newOpponentIDNumber != c.OpponentIDNumber)
	leadLawyerID := c.LeadLawyerID
	effectiveCo := c.CoLawyerIDs.ToUint64s()
	if reassignCo {
		effectiveCo = coLawyerIDs
	}
	if needCheck {
		check := s.buildConflictCheck(id, dedupLawyerIDs(leadLawyerID, effectiveCo), newOpponentName, newOpponentIDNumber)
		if check.HasConflict {
			s.logger.Warn(constants.LogCaseConflictBlocked, "stage", "update", "case_id", id,
				"reason", check.Reason, "conflict_cases", len(check.Cases))
			return nil, util.NewAppErrorWithDetail(constants.CodeLawyerConflict,
				"Case[id="+u64(id)+"] update rejected: lawyer conflict of interest: "+check.Reason, check)
		}
		if check.Warning != "" {
			s.logger.Info(constants.LogCaseConflictCheckSkip, "stage", "update", "case_id", id)
		}
	}
	if title != nil {
		c.Title = *title
	}
	if summary != nil {
		c.Summary = *summary
	}
	if reassignCo {
		c.CoLawyerIDs = jsonCoLawyers(effectiveCo)
	}
	if opponentChanged {
		c.OpponentName = newOpponentName
		c.OpponentIDNumber = newOpponentIDNumber
	}
	if err := s.repo.Update(c); err != nil {
		return nil, util.Wrap(err, "Case[id=%d] update save failed", id)
	}
	s.logger.Info(constants.LogCaseUpdateSuccess, "case_id", c.ID,
		"opponent_name", c.OpponentName, "opponent_id_number", c.OpponentIDNumber)
	check := s.buildConflictCheck(id, dedupLawyerIDs(c.LeadLawyerID, c.CoLawyerIDs.ToUint64s()),
		c.OpponentName, c.OpponentIDNumber)
	return &dto.CaseResult{Case: c, Conflicts: check, Warning: check.Warning}, nil
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
// 任一拟分配律师在未结案件中代理了证件号相同的对方当事人时，整次拒绝并列出冲突案号，
// 原主办/协作律师保持不变（先预检后落库）。管理员角色同样适用，不能绕过。
// 对方证件号缺失无法预检时只返回 warning，不阻断分配。
func (s *CaseService) Assign(id, leadLawyerID uint64, coLawyerIDs []uint64) (*dto.CaseResult, error) {
	c, err := s.repo.FindByID(id)
	if err != nil {
		return nil, util.Wrap(err, "Case[id=%d] assign find failed", id)
	}
	if _, err := s.userRepo.FindByID(leadLawyerID); err != nil {
		return nil, util.Wrap(err, "Case[id=%d] assign failed: lawyer not match", id)
	}
	lawyerIDs := dedupLawyerIDs(leadLawyerID, coLawyerIDs)
	check := s.buildConflictCheck(id, lawyerIDs, c.OpponentName, c.OpponentIDNumber)
	if check.HasConflict {
		s.logger.Warn(constants.LogCaseConflictBlocked, "stage", "assign", "case_id", id,
			"lead_lawyer_id", leadLawyerID, "reason", check.Reason, "conflict_cases", len(check.Cases))
		return nil, util.NewAppErrorWithDetail(constants.CodeLawyerConflict,
			"Case[id="+u64(id)+"] assign rejected by role[any]: lawyer conflict of interest: "+check.Reason, check)
	}
	if check.Warning != "" {
		s.logger.Info(constants.LogCaseConflictCheckSkip, "stage", "assign", "case_id", id)
	}
	c.LeadLawyerID = leadLawyerID
	if coLawyerIDs != nil {
		c.CoLawyerIDs = jsonCoLawyers(coLawyerIDs)
	}
	if err := s.repo.Update(c); err != nil {
		s.logger.Error(constants.LogCaseAssignFailed, "error", err.Error())
		return nil, util.Wrap(err, "Case[id=%d] assign save failed", id)
	}
	s.logger.Info(constants.LogCaseAssignSuccess, "case_id", c.ID, "lead_lawyer_id", leadLawyerID)
	return &dto.CaseResult{Case: c, Conflicts: check, Warning: check.Warning}, nil
}

// List 分页查询案件。
func (s *CaseService) List(page, pageSize int, caseType, status string, lawyerID uint64, startDate, endDate *time.Time) ([]model.Case, int64, error) {
	return s.repo.List(page, pageSize, caseType, status, lawyerID, startDate, endDate)
}

// Get 案件详情，并附带当前主办/协作律师针对对方证件号的实时冲突预检结果与冲突原因。
func (s *CaseService) Get(id uint64) (*dto.CaseResult, error) {
	c, err := s.repo.FindByID(id)
	if err != nil {
		return nil, util.Wrap(err, "Case[id=%d] get find failed", id)
	}
	lawyerIDs := dedupLawyerIDs(c.LeadLawyerID, c.CoLawyerIDs.ToUint64s())
	check := s.buildConflictCheck(id, lawyerIDs, c.OpponentName, c.OpponentIDNumber)
	return &dto.CaseResult{Case: c, Conflicts: check, Warning: check.Warning}, nil
}

// buildConflictCheck 执行律师利益冲突预检。
// 排除 excludeCaseID 本身后，查询对方证件号相同的全部未结案件，
// 只要案件的主办或协作律师命中 lawyerIDs 即构成冲突。证件号为空时不检查，仅返回 warning。
func (s *CaseService) buildConflictCheck(excludeCaseID uint64, lawyerIDs []uint64, opponentName, opponentIDNumber string) *dto.ConflictCheck {
	check := &dto.ConflictCheck{Cases: []dto.ConflictCase{}}
	idNumber := strings.TrimSpace(opponentIDNumber)
	if idNumber == "" {
		check.Warning = constants.MsgOpponentIDMissing
		return check
	}
	check.Checked = true
	wanted := make(map[uint64]struct{}, len(lawyerIDs))
	for _, id := range lawyerIDs {
		if id > 0 {
			wanted[id] = struct{}{}
		}
	}
	openCases, err := s.repo.ListOpenByOpponentIDNumber(idNumber, excludeCaseID)
	if err != nil {
		s.logger.Error("case conflict check query failed", "error", err.Error(), "opponent_id_number", idNumber)
		check.Checked = false
		check.Warning = constants.MsgOpponentIDMissing
		return check
	}
	nameByID := s.resolveLawyerNames(wanted)
	for i := range openCases {
		oc := &openCases[i]
		hit := make(map[uint64]struct{})
		if _, ok := wanted[oc.LeadLawyerID]; ok {
			hit[oc.LeadLawyerID] = struct{}{}
		}
		for _, coID := range oc.CoLawyerIDs.ToUint64s() {
			if _, ok := wanted[coID]; ok {
				hit[coID] = struct{}{}
			}
		}
		for lawyerID := range hit {
			check.Cases = append(check.Cases, dto.ConflictCase{
				LawyerID:         lawyerID,
				LawyerName:       nameByID[lawyerID],
				CaseID:           oc.ID,
				CaseNo:           oc.CaseNo,
				CaseTitle:        oc.Title,
				OpponentName:     oc.OpponentName,
				OpponentIDNumber: oc.OpponentIDNumber,
			})
		}
	}
	check.HasConflict = len(check.Cases) > 0
	if check.HasConflict {
		check.Reason = buildConflictReason(check.Cases)
	}
	return check
}

// resolveLawyerNames 批量解析律师显示姓名，查询失败时回退为 #id。
func (s *CaseService) resolveLawyerNames(ids map[uint64]struct{}) map[uint64]string {
	names := make(map[uint64]string, len(ids))
	for id := range ids {
		names[id] = fmt.Sprintf("#%d", id)
		u, err := s.userRepo.FindByID(id)
		if err != nil {
			continue
		}
		if strings.TrimSpace(u.RealName) != "" {
			names[id] = u.RealName
		} else {
			names[id] = u.Username
		}
	}
	return names
}

// buildConflictReason 将冲突明细汇总为可读的冲突原因文案，并列出全部冲突案号。
func buildConflictReason(cases []dto.ConflictCase) string {
	lawyerSet := make(map[string]struct{})
	caseNoSet := make(map[string]struct{})
	opponent := ""
	for _, c := range cases {
		lawyerSet[c.LawyerName] = struct{}{}
		caseNoSet[c.CaseNo] = struct{}{}
		if opponent == "" {
			opponent = c.OpponentName
			if opponent == "" {
				opponent = c.OpponentIDNumber
			}
		}
	}
	lawyers := sortedKeys(lawyerSet)
	caseNos := sortedKeys(caseNoSet)
	who := opponent
	if who == "" {
		who = "证件号相同的当事人"
	}
	return fmt.Sprintf("律师 %s 正在未结案件 %s 中代理对方当事人「%s」（证件号相同），存在利益冲突；本次操作整次拒绝，原分配保持不变，调整后可重试",
		strings.Join(lawyers, "、"), strings.Join(caseNos, "、"), who)
}

func sortedKeys(set map[string]struct{}) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// dedupLawyerIds 合并主办与协作律师并去重，保持稳定顺序。
func dedupLawyerIDs(lead uint64, co []uint64) []uint64 {
	seen := make(map[uint64]struct{}, len(co)+1)
	out := make([]uint64, 0, len(co)+1)
	if lead > 0 {
		seen[lead] = struct{}{}
		out = append(out, lead)
	}
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
	raw, _ := json.Marshal(ids)
	return model.CoLawyerJSON(raw)
}

func genCaseNo() string {
	return fmt.Sprintf("CY%d%04d", time.Now().Year(), time.Now().UnixNano()%10000)
}

func u64(v uint64) string {
	return fmt.Sprintf("%d", v)
}
