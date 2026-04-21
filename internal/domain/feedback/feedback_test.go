package feedback_test

import (
	"testing"
	"codeRunner-siwu/internal/domain/feedback"
)

func TestFeedback_ValidTypes(t *testing.T) {
	for _, typ := range []string{"bug", "suggestion", "other"} {
		f := feedback.Feedback{Type: typ, Content: "这是一条有效的反馈内容"}
		if err := f.Validate(); err != nil {
			t.Errorf("type %q should be valid, got %v", typ, err)
		}
	}
}

func TestFeedback_InvalidType(t *testing.T) {
	f := feedback.Feedback{Type: "spam", Content: "这是一条有效的反馈内容"}
	if err := f.Validate(); err != feedback.ErrInvalidType {
		t.Errorf("expected ErrInvalidType, got %v", err)
	}
}

func TestFeedback_ContentTooShort(t *testing.T) {
	f := feedback.Feedback{Type: "bug", Content: "短"}
	if err := f.Validate(); err != feedback.ErrInvalidContent {
		t.Errorf("expected ErrInvalidContent, got %v", err)
	}
}

func TestFeedback_ContentTooLong(t *testing.T) {
	f := feedback.Feedback{Type: "bug", Content: string(make([]byte, 2001))}
	if err := f.Validate(); err != feedback.ErrInvalidContent {
		t.Errorf("expected ErrInvalidContent, got %v", err)
	}
}

func TestFeedback_ContentTrimmed(t *testing.T) {
	f := feedback.Feedback{Type: "bug", Content: "   短   "}
	if err := f.Validate(); err != feedback.ErrInvalidContent {
		t.Errorf("expected ErrInvalidContent after trim, got %v", err)
	}
}

func TestFeedback_ContactTooLong(t *testing.T) {
	f := feedback.Feedback{Type: "bug", Content: "这是一条有效的反馈内容", Contact: string(make([]byte, 101))}
	if err := f.Validate(); err != feedback.ErrInvalidContact {
		t.Errorf("expected ErrInvalidContact, got %v", err)
	}
}

func TestFeedback_ValidOptionalContact(t *testing.T) {
	f := feedback.Feedback{Type: "bug", Content: "这是一条有效的反馈内容", Contact: "me@example.com"}
	if err := f.Validate(); err != nil {
		t.Errorf("valid contact should pass, got %v", err)
	}
}
