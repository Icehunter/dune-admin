package main

import (
	"context"
	"reflect"
	"testing"
)

func TestValidateContractMutationInput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		accountID int64
		contracts []string
		wantError string
	}{
		{name: "valid", accountID: 10, contracts: []string{"DA_CT_A"}},
		{name: "missing-account", accountID: 0, contracts: []string{"DA_CT_A"}, wantError: "account ID required"},
		{name: "missing-contracts", accountID: 10, contracts: nil, wantError: "at least one contract required"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := validateContractMutationInput(tt.accountID, tt.contracts)
			if tt.wantError == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || err.Error() != tt.wantError {
				t.Fatalf("expected error %q, got %v", tt.wantError, err)
			}
		})
	}
}

func TestBuildContractRemovalSet(t *testing.T) {
	t.Parallel()

	originalTagsData := tagsData
	tagsData = tagsDataFile{
		ContractAliases: map[string]string{
			"shortA": "DA_CT_A",
		},
		ContractTags: map[string][]string{
			"DA_CT_A": {"Tag.A", "Tag.B"},
			"DA_CT_B": {"Tag.B", "Tag.C"},
		},
		ContractSkillGrants: map[string][]string{
			"DA_CT_A": {"Skills.Key.A", "Skills.Key.B"},
			"DA_CT_B": {"Skills.Key.B", "Skills.Key.C"},
		},
	}
	t.Cleanup(func() { tagsData = originalTagsData })

	set, err := buildContractRemovalSet([]string{"shortA", "DA_CT_B", "shortA"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wantResolved := []string{"DA_CT_A", "DA_CT_B", "DA_CT_A"}
	if !reflect.DeepEqual(set.resolvedNames, wantResolved) {
		t.Fatalf("unexpected resolved names: %#v", set.resolvedNames)
	}

	wantTags := []string{"Tag.A", "Tag.B", "Tag.C"}
	if !reflect.DeepEqual(set.removeTags, wantTags) {
		t.Fatalf("unexpected remove tags: %#v", set.removeTags)
	}

	wantSkills := []string{"Skills.Key.A", "Skills.Key.B", "Skills.Key.C"}
	if !reflect.DeepEqual(set.removeSkills, wantSkills) {
		t.Fatalf("unexpected remove skills: %#v", set.removeSkills)
	}
}

func TestBuildContractRemovalSet_UnknownContract(t *testing.T) {
	t.Parallel()

	originalTagsData := tagsData
	tagsData = tagsDataFile{
		ContractAliases: map[string]string{},
		ContractTags:    map[string][]string{},
	}
	t.Cleanup(func() { tagsData = originalTagsData })

	_, err := buildContractRemovalSet([]string{"missing"})
	if err == nil {
		t.Fatal("expected unknown contract error")
	}
}

func TestContractBatchSummary(t *testing.T) {
	t.Parallel()

	if got := contractBatchSummary([]string{"DA_CT_SINGLE"}); got != "DA_CT_SINGLE" {
		t.Fatalf("unexpected single summary: %q", got)
	}
	if got := contractBatchSummary([]string{"A", "B"}); got != "2 contracts" {
		t.Fatalf("unexpected multi summary: %q", got)
	}
}

func TestRemoveContractTags_NoTagsIsNoop(t *testing.T) {
	t.Parallel()

	if err := removeContractTags(context.Background(), 123, nil); err != nil {
		t.Fatalf("expected no-op nil tags, got %v", err)
	}
}

func TestStripContractSkillBlocks_NoopCases(t *testing.T) {
	t.Parallel()

	if stripped, err := stripContractSkillBlocks(context.Background(), 0, []string{"Skills.Key.A"}); err != nil || stripped != 0 {
		t.Fatalf("expected pawn 0 no-op, stripped=%d err=%v", stripped, err)
	}
	if stripped, err := stripContractSkillBlocks(context.Background(), 10, nil); err != nil || stripped != 0 {
		t.Fatalf("expected empty skills no-op, stripped=%d err=%v", stripped, err)
	}
}
