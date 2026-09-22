//go:build sqlite_integration

package handler_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"cylawcase/internal/config"
	"cylawcase/internal/constants"
	"cylawcase/internal/dto"
	"cylawcase/internal/handler"
	"cylawcase/internal/middleware"
	"cylawcase/internal/model"
	"cylawcase/internal/repository"
	"cylawcase/internal/service"
	"cylawcase/internal/util"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupConflictRouter(t *testing.T) (*gin.Engine, *gorm.DB, string, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	f, err := os.CreateTemp("", "http-*.db")
	if err != nil {
		t.Fatal(err)
	}
	db, err := gorm.Open(sqlite.Open(f.Name()), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.Client{}, &model.Case{}); err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	lawyer := &model.User{Username: "zhang", RealName: "张律师", Role: constants.RoleLawyer}
	other := &model.User{Username: "wang", RealName: "王律师", Role: constants.RoleLawyer}
	admin := &model.User{Username: "boss", RealName: "管理员", Role: constants.RoleAdmin}
	for _, u := range []*model.User{lawyer, other, admin} {
		if err := db.Create(u).Error; err != nil {
			t.Fatal(err)
		}
	}
	client := &model.Client{Name: "客户甲", IDNumber: "C1"}
	if err := db.Create(client).Error; err != nil {
		t.Fatal(err)
	}
	// 未结案件：张律师主办，对方 ID-OPP
	if err := db.Create(&model.Case{
		CaseNo: "CY-EXISTING", Title: "存量未结案", CaseType: constants.CaseTypeCivil,
		Status: constants.CaseStatusInvestigating, ClientID: client.ID, LeadLawyerID: lawyer.ID,
		CoLawyerIDs: model.CoLawyerJSON("[]"), OpponentName: "周立波", OpponentIDNumber: "ID-OPP",
	}).Error; err != nil {
		t.Fatal(err)
	}
	caseRepo := repository.NewCaseRepository(db)
	caseSvc := service.NewCaseService(caseRepo, repository.NewClientRepository(db), repository.NewUserRepository(db), logger)
	caseH := handler.NewCaseHandler(caseSvc, logger)

	// 颁发两个 token：律师（用于普通调用）、管理员（验证不能绕过）
	cfg := &config.Config{JWTSecret: "secret", JWTExpireHours: 1}
	lawyerToken := mustToken(t, cfg, lawyer.ID, lawyer.Username, constants.RoleLawyer)
	adminToken := mustToken(t, cfg, admin.ID, admin.Username, constants.RoleAdmin)

	r := gin.New()
	g := r.Group("/api/v1")
	g.Use(middleware.JWTConfig(cfg), middleware.AuthRequired(cfg))
	g.POST("/cases", caseH.Create)
	g.POST("/cases/:id/assign", caseH.Assign)
	g.GET("/cases/:id", caseH.Get)
	return r, db, lawyerToken, adminToken
}

func mustToken(t *testing.T, cfg *config.Config, id uint64, name, role string) string {
	t.Helper()
	tok, err := util.GenerateToken(cfg.JWTSecret, cfg.JWTExpireDuration(), id, name, role)
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

func doJSON(t *testing.T, r http.Handler, method, path, token string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestHTTPCreateConflictAndAdminCannotBypass(t *testing.T) {
	r, _, lawyerToken, adminToken := setupConflictRouter(t)

	payload := map[string]any{
		"client_id": 1, "lead_lawyer_id": 1, "title": "冲突新案",
		"case_type": "civil", "opponent_name": "周立波", "opponent_id_number": "ID-OPP",
	}

	// 律师创建被拒：409 + 结构化冲突明细
	w := doJSON(t, r, http.MethodPost, "/api/v1/cases", lawyerToken, payload)
	if w.Code != http.StatusConflict {
		t.Fatalf("lawyer create want 409, got %d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			Reason string             `json:"reason"`
			Cases  []dto.ConflictCase `json:"cases"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Code != constants.CodeLawyerConflict {
		t.Fatalf("want code %d, got %d", constants.CodeLawyerConflict, resp.Code)
	}
	if len(resp.Data.Cases) == 0 || resp.Data.Cases[0].CaseNo != "CY-EXISTING" {
		t.Fatalf("response must list CY-EXISTING, got %s", w.Body.String())
	}

	// 管理员同样被拒，不能绕过
	w2 := doJSON(t, r, http.MethodPost, "/api/v1/cases", adminToken, payload)
	if w2.Code != http.StatusConflict {
		t.Fatalf("admin must NOT bypass conflict check, got %d body=%s", w2.Code, w2.Body.String())
	}

	// 资料缺失（证件号空）：创建成功 200，响应 message 为提示文案
	payload["opponent_id_number"] = ""
	w3 := doJSON(t, r, http.MethodPost, "/api/v1/cases", lawyerToken, payload)
	if w3.Code != http.StatusOK {
		t.Fatalf("missing opponent id must not block, got %d body=%s", w3.Code, w3.Body.String())
	}
	var okResp struct {
		Message string `json:"message"`
	}
	_ = json.Unmarshal(w3.Body.Bytes(), &okResp)
	if okResp.Message != constants.MsgOpponentIDMissing {
		t.Errorf("want warning message %q, got %q", constants.MsgOpponentIDMissing, okResp.Message)
	}
}
