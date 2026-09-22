package service

import (
	"strings"
	"testing"

	"cylawcase/internal/dto"
	"cylawcase/internal/model"
)

func TestDedupLawyerIDs(t *testing.T) {
	cases := []struct {
		name string
		lead uint64
		co   []uint64
		want []uint64
	}{
		{"only lead", 2, nil, []uint64{2}},
		{"lead plus co", 2, []uint64{3, 4}, []uint64{2, 3, 4}},
		{"co duplicates lead", 2, []uint64{2, 3}, []uint64{2, 3}},
		{"co duplicates each other", 2, []uint64{3, 3, 4, 4}, []uint64{2, 3, 4}},
		{"zero ids dropped", 2, []uint64{0, 3}, []uint64{2, 3}},
		{"empty", 0, nil, []uint64{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := dedupLawyerIDs(tc.lead, tc.co)
			if len(got) != len(tc.want) {
				t.Fatalf("dedupLawyerIDs(%d,%v) = %v, want %v", tc.lead, tc.co, got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("dedupLawyerIDs(%d,%v) = %v, want %v", tc.lead, tc.co, got, tc.want)
				}
			}
		})
	}
}

func TestBuildConflictReason(t *testing.T) {
	cases := []dto.ConflictCase{
		{LawyerName: "张律师", CaseNo: "CY20260001", OpponentName: "周立波", OpponentIDNumber: "440300198507073210"},
		{LawyerName: "张律师", CaseNo: "CY20260005", OpponentName: "周立波", OpponentIDNumber: "440300198507073210"},
		{LawyerName: "李律师", CaseNo: "CY20260001", OpponentName: "周立波", OpponentIDNumber: "440300198507073210"},
	}
	reason := buildConflictReason(cases)
	for _, want := range []string{"张律师", "李律师", "CY20260001", "CY20260005", "周立波", "利益冲突", "整次拒绝"} {
		if !strings.Contains(reason, want) {
			t.Errorf("conflict reason %q missing %q", reason, want)
		}
	}
}

func TestCoLawyerJSONToUint64s(t *testing.T) {
	if ids := model.CoLawyerJSON(nil).ToUint64s(); len(ids) != 0 {
		t.Errorf("nil co lawyers should be empty, got %v", ids)
	}
	if ids := model.CoLawyerJSON("[]").ToUint64s(); len(ids) != 0 {
		t.Errorf("empty co lawyers should be empty, got %v", ids)
	}
	ids := model.CoLawyerJSON(`[3,4]`).ToUint64s()
	if len(ids) != 2 || ids[0] != 3 || ids[1] != 4 {
		t.Errorf("co lawyers parse = %v, want [3 4]", ids)
	}
}
