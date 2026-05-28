package main

import "testing"

func TestParseINILines_FiltersSchemaAndPreservesArrayPrefixes(t *testing.T) {
	t.Parallel()

	content := `
; comment
[SectionOne]
Known=1
Unknown = 2
+Known=3
-Known=4
NoEqualsHere

[SectionTwo]
Another=abc
`
	schemaKeys := map[string]bool{"SectionOne|Known": true}

	got := parseINILines(content, "userGame", schemaKeys)
	if len(got) != 2 {
		t.Fatalf("expected 2 sections, got %d", len(got))
	}
	if got[0].Section != "SectionOne" || got[0].Source != "userGame" {
		t.Fatalf("unexpected first section metadata: %+v", got[0])
	}
	if len(got[0].Lines) != 3 {
		t.Fatalf("expected 3 lines in SectionOne, got %d", len(got[0].Lines))
	}
	if got[0].Lines[0] != (RawLine{Key: "Unknown", Value: "2"}) {
		t.Fatalf("unexpected first raw line: %+v", got[0].Lines[0])
	}
	if got[0].Lines[1] != (RawLine{Prefix: "+", Key: "Known", Value: "3"}) {
		t.Fatalf("unexpected +array raw line: %+v", got[0].Lines[1])
	}
	if got[0].Lines[2] != (RawLine{Prefix: "-", Key: "Known", Value: "4"}) {
		t.Fatalf("unexpected -array raw line: %+v", got[0].Lines[2])
	}

	if got[1].Section != "SectionTwo" || len(got[1].Lines) != 1 {
		t.Fatalf("unexpected second section: %+v", got[1])
	}
	if got[1].Lines[0] != (RawLine{Key: "Another", Value: "abc"}) {
		t.Fatalf("unexpected second section line: %+v", got[1].Lines[0])
	}
}

func TestParseINILines_DropsSectionsWithoutRawLines(t *testing.T) {
	t.Parallel()

	content := `
[OnlySchema]
Known=1
[HasRaw]
Unknown=2
`
	schemaKeys := map[string]bool{"OnlySchema|Known": true}

	got := parseINILines(content, "defaultGame", schemaKeys)
	if len(got) != 1 {
		t.Fatalf("expected only one section after compaction, got %d", len(got))
	}
	if got[0].Section != "HasRaw" {
		t.Fatalf("expected HasRaw section to remain, got %q", got[0].Section)
	}
}
