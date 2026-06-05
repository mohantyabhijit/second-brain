package langfuse

import "testing"

func TestCompileTextPrompt(t *testing.T) {
	template := "Digest {{digest_date}}\n{{ input_json }}"
	got := CompileTextPrompt(template, map[string]string{
		"digest_date": "2026-06-05",
		"input_json":  `{"items":1}`,
	})
	want := "Digest 2026-06-05\n{\"items\":1}"
	if got != want {
		t.Fatalf("compile prompt mismatch\nwant: %q\n got: %q", want, got)
	}
}
