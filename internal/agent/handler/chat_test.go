package handler

import (
	"strings"
	"testing"
)

func TestBuildInstruction_Nil(t *testing.T) {
	result := buildInstruction(nil)
	if result != "" {
		t.Errorf("expected empty string for nil articleCtx, got %q", result)
	}
}

func TestBuildInstruction_NoContent(t *testing.T) {
	ctx := &articleCtx{}
	result := buildInstruction(ctx)
	if !strings.Contains(result, "coding assistant") {
		t.Errorf("expected base instruction, got %q", result)
	}
}

func TestBuildInstruction_WithArticleContent(t *testing.T) {
	ctx := &articleCtx{
		ArticleID:      "art-1",
		ArticleContent: "This article explains Go interfaces.",
	}
	result := buildInstruction(ctx)
	if !strings.Contains(result, "Article Context") {
		t.Errorf("expected Article Context section, got %q", result)
	}
	if !strings.Contains(result, "Go interfaces") {
		t.Errorf("expected article content in instruction, got %q", result)
	}
}

func TestBuildInstruction_WithCodeBlocks(t *testing.T) {
	ctx := &articleCtx{
		ArticleContent: "Sample article.",
		CodeBlocks: []codeBlock{
			{Language: "go", Code: "fmt.Println(\"hello\")"},
			{Language: "python", Code: "print('hello')"},
		},
	}
	result := buildInstruction(ctx)
	if !strings.Contains(result, "Code Blocks") {
		t.Errorf("expected Code Blocks section, got %q", result)
	}
	if !strings.Contains(result, "Block 1 (go)") {
		t.Errorf("expected Block 1 (go), got %q", result)
	}
	if !strings.Contains(result, "Block 2 (python)") {
		t.Errorf("expected Block 2 (python), got %q", result)
	}
	if !strings.Contains(result, "fmt.Println") {
		t.Errorf("expected go code in instruction, got %q", result)
	}
}

func TestBuildInstruction_ContainsToolInstructions(t *testing.T) {
	ctx := &articleCtx{ArticleContent: "test"}
	result := buildInstruction(ctx)
	if !strings.Contains(result, "Instructions") {
		t.Errorf("expected Instructions section, got %q", result)
	}
}
