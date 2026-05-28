package main

import "testing"

func TestStripKeysFromContent(t *testing.T) {
	t.Parallel()

	content := `[Sec]
Foo=1
+Foo=2
-Foo=3
Bar=9
[Other]
Foo=4
`

	owned := map[string]map[string]bool{
		"Sec": {"Foo": true},
	}
	got := stripKeysFromContent(content, owned)
	want := `[Sec]
Bar=9
[Other]
Foo=4
`
	if got != want {
		t.Fatalf("unexpected stripped content\nwant:\n%q\ngot:\n%q", want, got)
	}
}

func TestStripKeysFromContent_PrefixedOwnership(t *testing.T) {
	t.Parallel()

	content := `[Sec]
Foo=1
+Foo=2
-Foo=3
`

	owned := map[string]map[string]bool{
		"Sec": {"+Foo": true},
	}
	got := stripKeysFromContent(content, owned)
	want := `[Sec]
Foo=1
-Foo=3
`
	if got != want {
		t.Fatalf("unexpected prefixed strip result\nwant:\n%q\ngot:\n%q", want, got)
	}
}

func TestStripKeysFromContent_PreservesCommentsAndInvalidLines(t *testing.T) {
	t.Parallel()

	content := `[Sec]
; comment
# hash comment
NoEquals
Owned=1
`

	owned := map[string]map[string]bool{
		"Sec": {"Owned": true},
	}
	got := stripKeysFromContent(content, owned)
	want := `[Sec]
; comment
# hash comment
NoEquals
`
	if got != want {
		t.Fatalf("unexpected comment preservation result\nwant:\n%q\ngot:\n%q", want, got)
	}
}
