package tools

import (
	"context"
	"testing"
)

func TestNormalizeLanguage(t *testing.T) {
	cases := map[string]string{
		"go":         "golang",
		"Go":         "golang",
		"golang":     "golang",
		"python":     "python",
		"Python":     "python",
		"py":         "python",
		"javascript": "javascript",
		"js":         "javascript",
		"JavaScript": "javascript",
		"java":       "java",
		"Java":       "java",
		"c":          "c",
		"C":          "c",
	}
	for in, want := range cases {
		got, err := normalizeLanguage(in)
		if err != nil {
			t.Fatalf("normalizeLanguage(%q): %v", in, err)
		}
		if got != want {
			t.Fatalf("normalizeLanguage(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNormalizeLanguage_Unsupported(t *testing.T) {
	unsupported := []string{"cpp", "c++", "rust", "ruby", ""}
	for _, lang := range unsupported {
		_, err := normalizeLanguage(lang)
		if err == nil {
			t.Fatalf("expected error for %q", lang)
		}
	}
}

func TestProposeExecutionTool_Info(t *testing.T) {
	tool := NewProposeExecutionTool()
	info, err := tool.Info(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if info.Name != "propose_execution" {
		t.Fatalf("tool name = %q, want propose_execution", info.Name)
	}
}
