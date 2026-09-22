package service

import (
	"strings"
	"testing"

	"cylawcase/internal/constants"
	"cylawcase/internal/model"
)

func TestDedupeLawyerIDs(t *testing.T) {
	got := dedupeLawyerIDs(2, []uint64{3, 2, 0, 4, 3})
	want := []uint64{2, 3, 4}
	if len(got) != len(want) {
		t.Fatalf("dedupeLawyerIDs len = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("dedupeLawyerIDs = %v, want %v", got, want)
		}
	}
}

func TestParseCoLawyerIDs(t *testing.T) {
	if ids := parseCoLawyerIDs(nil); len(ids) != 0 {
		t.Errorf("nil raw should yield empty slice, got %v", ids)
	}
	if ids := parseCoLawyerIDs(model.CoLawyerJSON(`[7,8]`)); len(ids) != 2 || ids[0] != 7 || ids[1] != 8 {
		t.Errorf("valid json unmarshal failed, got %v", ids)
	}
	if ids := parseCoLawyerIDs(model.CoLawyerJSON(`not-json`)); len(ids) != 0 {
		t.Errorf("invalid json should yield empty slice, got %v", ids)
	}
	if containsLawyerID([]uint64{1, 2}, 3) {
		t.Error("3 should not be in [1 2]")
	}
	if !containsLawyerID([]uint64{1, 2}, 2) {
		t.Error("2 should be in [1 2]")
	}
}

func TestBuildConflictMessage(t *testing.T) {
	c := &model.Case{ID: 9, OpponentName: "周丽华", OpponentIDNumber: "440300198505056789"}
	conflicts := []LawyerConflict{
		{LawyerID: 2, LawyerName: "张律师", ConflictCaseNos: []string{"CY20260002", "CY20260004"}},
	}
	msg := buildConflictMessage(9, c, conflicts)
	for _, fragment := range []string{
		"Case[id=9]",
		"assign rejected",
		"role=lawyer",
		"周丽华",
		"440300198505056789",
		"张律师",
		"CY20260002",
		"CY20260004",
	} {
		if !strings.Contains(msg, fragment) {
			t.Errorf("conflict message %q should contain %q", msg, fragment)
		}
	}
}

func TestLawyerConflictError(t *testing.T) {
	err := &LawyerConflictError{Message: constants.MsgLawyerConflict}
	if got, ok := IsLawyerConflictError(err); !ok || got.Message != constants.MsgLawyerConflict {
		t.Error("IsLawyerConflictError should match LawyerConflictError")
	}
	if _, ok := IsLawyerConflictError(errInvalidCase()); ok {
		t.Error("IsLawyerConflictError should not match generic error")
	}
}

func errInvalidCase() error { return &simpleErr{"not a conflict"} }

type simpleErr struct{ s string }

func (e *simpleErr) Error() string { return e.s }
