package main

import "testing"

func TestSpiceVisionNodeIDs(t *testing.T) {
	t.Parallel()

	// DA_MQ_FindTheFremen and any of its sub-nodes must trigger spice vision.
	cases := []struct {
		nodeID string
		want   bool
	}{
		{"DA_MQ_FindTheFremen", true},
		{"DA_MQ_FindTheFremen.FourthTest", true},
		{"DA_MQ_FindTheFremen.FourthTest.FourthQuestion.CompleteFourthTest", true},
		{"DA_MQ_FindTheFremen.Epilogue", true},
		{"DA_MQ_ANewBeginning", false},
		{"DA_MQ_ANewBeginning.Aql No 1.FabricateStillsuit.Equip the Stillsuit", false},
		{"DA_SQ_VermiliusGap", false},
		{"DA_FQ_ClimbTheRanks", false},
	}

	for _, tc := range cases {
		got := nodeIDTriggersSpiceVision(tc.nodeID)
		if got != tc.want {
			t.Errorf("nodeIDTriggersSpiceVision(%q) = %v, want %v", tc.nodeID, got, tc.want)
		}
	}
}

func TestSpiceVisionSQL(t *testing.T) {
	t.Parallel()

	// Verify the SQL snippet is well-formed and targets the right JSONB path.
	sql := spiceVisionEnableSQL
	for _, substr := range []string{
		"FSpiceAddictionComponent",
		"SpiceVisionEnabledStatus",
		"FullyEnabled",
		"DuneCharacter",
	} {
		if !containsSubstring(sql, substr) {
			t.Errorf("spiceVisionEnableSQL missing %q", substr)
		}
	}
}

func containsSubstring(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsSubstringHelper(s, sub))
}

func containsSubstringHelper(s, sub string) bool {
	for i := range s {
		if i+len(sub) <= len(s) && s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
