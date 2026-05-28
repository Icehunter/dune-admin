package main

import "testing"

func TestStripEmptySections(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "empty content",
			content: "",
			want:    "",
		},
		{
			name:    "removes section with only blank body",
			content: "[Keep]\nKey=1\n\n[Remove]\n\n\n[AlsoKeep]\n;comment\n",
			want:    "[Keep]\nKey=1\n\n[AlsoKeep]\n;comment\n",
		},
		{
			name:    "preserves sections with comments or values",
			content: "[Commented]\n; docs\n\n[Valued]\n+Array=1\n\n[Blank]\n\n",
			want:    "[Commented]\n; docs\n\n[Valued]\n+Array=1\n",
		},
		{
			name:    "keeps non-section preface lines",
			content: "; header\n\n[Remove]\n\n[Keep]\nValue=1\n",
			want:    "; header\n\n[Keep]\nValue=1\n",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			if got := stripEmptySections(tt.content); got != tt.want {
				t.Fatalf("unexpected output\nwant:\n%q\ngot:\n%q", tt.want, got)
			}
		})
	}
}
