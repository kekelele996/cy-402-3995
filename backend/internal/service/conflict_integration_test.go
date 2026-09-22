//go:build sqlite_integration

package service

import (
	"encoding/json"
	"log/slog"
	"os"
	"testing"

	"cylawcase/internal/constants"
	"cylawcase/internal/dto"
	"cylawcase/internal/model"
	"cylawcase/internal/repository"
	"cylawcase/internal/util"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type fixture struct {
	svc        *CaseService
	openCase   *model.Case
	lawyer2    *model.User
	lawyer4    *model.User
	assistant3 *model.User
	client1    *model.Client
}

func newConflictTestDB(t *testing.T) (*gorm.DB, func()) {
	t.Helper()
	f, err := os.CreateTemp("", "conflict-*.db")
	if err != nil {
		t.Fatal(err)
	}
	path := f.Name()
	f.Close()
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.Client{}, &model.Case{}); err != nil {
		t.Fatal(err)
	}
	return db, func() { os.Remove(path) }
}

func seedConflictFixture(t *testing.T, db *gorm.DB) *fixture {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	lawyer2 := &model.User{Username: "lawyer", RealName: "张律师", Role: constants.RoleLawyer}
	lawyer4 := &model.User{Username: "lawyer2", RealName: "王律师", Role: constants.RoleLawyer}
	assistant3 := &model.User{Username: "assistant", RealName: "李助理", Role: constants.RoleAssistant}
	for _, u := range []*model.User{lawyer2, lawyer4, assistant3} {
		if err := db.Create(u).Error; err != nil {
			t.Fatal(err)
		}
	}
	client1 := &model.Client{Name: "深圳华信", IDNumber: "91440300MA1"}
	if err := db.Create(client1).Error; err != nil {
		t.Fatal(err)
	}
	// 未结案件 CY-OPEN：张律师主办，李助理协作，对方证件号 ID-OPP
	openCase := &model.Case{
		CaseNo: "CY-OPEN", Title: "在审案件", CaseType: constants.CaseTypeCivil,
		Status: constants.CaseStatusHearing, ClientID: client1.ID, LeadLawyerID: lawyer2.ID,
		CoLawyerIDs:  jsonCoLawyers([]uint64{assistant3.ID}),
		OpponentName: "周立波", OpponentIDNumber: "ID-OPP",
	}
	if err := db.Create(openCase).Error; err != nil {
		t.Fatal(err)
	}
	// 已结案件 CY-CLOSED：张律师主办，相同对方证件号，不应触发冲突
	closedCase := &model.Case{
		CaseNo: "CY-CLOSED", Title: "已结案件", CaseType: constants.CaseTypeCivil,
		Status: constants.CaseStatusClosed, ClientID: client1.ID, LeadLawyerID: lawyer2.ID,
		OpponentName: "周立波", OpponentIDNumber: "ID-OPP",
	}
	if err := db.Create(closedCase).Error; err != nil {
		t.Fatal(err)
	}
	svc := NewCaseService(repository.NewCaseRepository(db),
		repository.NewClientRepository(db), repository.NewUserRepository(db), logger)
	return &fixture{svc, openCase, lawyer2, lawyer4, assistant3, client1}
}

func conflictDetail(t *testing.T, err error) *dto.ConflictCheck {
	t.Helper()
	var appErr *util.AppError
	if !asAppError(err, &appErr) {
		t.Fatalf("want *util.AppError, got %#v", err)
	}
	if appErr.Code != constants.CodeLawyerConflict {
		t.Fatalf("want CodeLawyerConflict, got %d", appErr.Code)
	}
	check, ok := appErr.Detail.(*dto.ConflictCheck)
	if !ok {
		t.Fatalf("want detail *dto.ConflictCheck, got %T", appErr.Detail)
	}
	return check
}

func asAppError(err error, target **util.AppError) bool {
	for err != nil {
		if ae, ok := err.(*util.AppError); ok {
			*target = ae
			return true
		}
		type unwrapper interface{ Unwrap() error }
		u, ok := err.(unwrapper)
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}

func TestConflictFlow(t *testing.T) {
	db, cleanup := newConflictTestDB(t)
	defer cleanup()
	fx := seedConflictFixture(t, db)
	svc := fx.svc

	// 王律师与 ID-OPP 无关，创建应成功
	res, err := svc.Create(fx.client1.ID, fx.lawyer4.ID, "新案", constants.CaseTypeCivil, "", nil, nil, "周立波", "ID-OPP")
	if err != nil {
		t.Fatalf("create with unrelated lawyer should pass: %v", err)
	}
	newCase := res.Case.(*model.Case)

	// 张律师在未结案件 CY-OPEN 中代理同一对方：分配为主办必须整次拒绝
	_, err = svc.Assign(newCase.ID, fx.lawyer2.ID, nil)
	check := conflictDetail(t, err)
	caseNos := map[string]bool{}
	for _, cc := range check.Cases {
		caseNos[cc.CaseNo] = true
	}
	if !caseNos["CY-OPEN"] {
		t.Errorf("conflict must list CY-OPEN, got %s", mustJSON(check))
	}
	if caseNos["CY-CLOSED"] {
		t.Errorf("closed case CY-CLOSED must not conflict, got %s", mustJSON(check))
	}
	if check.Reason == "" || !containsStr(check.Reason, "CY-OPEN") {
		t.Errorf("conflict reason must mention case numbers, got %q", check.Reason)
	}
	// 原分配保持不变
	c, err := svc.repo.FindByID(newCase.ID)
	if err != nil {
		t.Fatal(err)
	}
	if c.LeadLawyerID != fx.lawyer4.ID {
		t.Fatalf("lead lawyer must remain %d, got %d", fx.lawyer4.ID, c.LeadLawyerID)
	}

	// 协作律师命中（李助理是 CY-OPEN 协作律师）同样拒绝，且原分配不变
	_, err = svc.Assign(newCase.ID, fx.lawyer4.ID, []uint64{fx.assistant3.ID})
	conflictDetail(t, err)
	c, _ = svc.repo.FindByID(newCase.ID)
	if c.LeadLawyerID != fx.lawyer4.ID || len(c.CoLawyerIDs.ToUint64s()) != 0 {
		t.Fatal("original assignment must remain after rejected assign")
	}

	// 创建案件时同样预检：张律师直接建新案也应被拒绝
	if _, err := svc.Create(fx.client1.ID, fx.lawyer2.ID, "又一案", constants.CaseTypeCivil, "", nil, nil, "周立波", "ID-OPP"); err == nil {
		t.Fatal("create with conflicting lead lawyer must be rejected")
	}

	// 更新案件：把对方证件号改成与 CY-OPEN 相同（当前为王律师，但协作改成张律师会冲突）
	if _, err := svc.Update(newCase.ID, nil, nil, []uint64{fx.lawyer2.ID}, nil, nil); err == nil {
		t.Fatal("update adding conflicting co lawyer must be rejected")
	}
	c, _ = svc.repo.FindByID(newCase.ID)
	if len(c.CoLawyerIDs.ToUint64s()) != 0 {
		t.Fatal("rejected update must keep original co lawyers")
	}

	// 调整对方证件号后重试分配，应成功
	if _, err := svc.Update(newCase.ID, nil, nil, nil, nil, strPtr("ID-OTHER")); err != nil {
		t.Fatalf("update opponent id should succeed: %v", err)
	}
	if _, err := svc.Assign(newCase.ID, fx.lawyer2.ID, []uint64{fx.assistant3.ID}); err != nil {
		t.Fatalf("assign after adjustment should succeed: %v", err)
	}

	// 对方证件号缺失：只提示不阻断创建
	res2, err := svc.Create(fx.client1.ID, fx.lawyer2.ID, "缺证件案", constants.CaseTypeCivil, "", nil, nil, "无名氏", "")
	if err != nil {
		t.Fatalf("missing opponent id must not block create: %v", err)
	}
	if res2.Warning != constants.MsgOpponentIDMissing {
		t.Errorf("want missing-id warning, got %q", res2.Warning)
	}
	// 证件号缺失时分配也只提示不阻断
	assignRes, err := svc.Assign(res2.Case.(*model.Case).ID, fx.lawyer2.ID, nil)
	if err != nil {
		t.Fatalf("missing opponent id must not block assign: %v", err)
	}
	if assignRes.Warning == "" {
		t.Error("assign with missing opponent id should carry warning")
	}

	// 案件详情自身排除：CY-OPEN 当前律师（张律师+李助理）不应与自身报冲突
	detail, err := svc.Get(fx.openCase.ID)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Conflicts == nil || detail.Conflicts.HasConflict {
		t.Fatalf("self must be excluded, got %s", mustJSON(detail.Conflicts))
	}
	// 新案已调整为 ID-OTHER 且分配了张律师/李助理，详情也不应冲突
	got, err := svc.Get(newCase.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Conflicts != nil && got.Conflicts.HasConflict {
		t.Fatalf("adjusted case should have no conflict, got %s", mustJSON(got.Conflicts))
	}

	// 已结案件 CY-CLOSED 详情不应与 CY-OPEN 冲突（它本身已结，张律师仍主办）
	// 注意：CY-CLOSED 的律师在 CY-OPEN 中代理相同对方，但 CY-CLOSED 自己是“当前案件”，
	// 预检排查的是律师的“其他未结案件”，CY-OPEN 为未结案件且对方相同 -> 应提示冲突。
	closedList, _ := svc.repo.ListOpenByOpponentIDNumber("ID-OPP", 0)
	openNos := map[string]bool{}
	for _, cc := range closedList {
		openNos[cc.CaseNo] = true
	}
	if !openNos["CY-OPEN"] || openNos["CY-CLOSED"] {
		t.Fatalf("open-case query must include CY-OPEN only, got %v", openNos)
	}
}

func strPtr(s string) *string { return &s }

func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	}())
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
