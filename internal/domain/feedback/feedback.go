package feedback

import "strings"

var validTypes = map[string]bool{
	"bug": true, "suggestion": true, "other": true,
}

type Feedback struct {
	Type    string
	Content string
	Contact string
}

func (f *Feedback) Validate() error {
	if !validTypes[f.Type] {
		return ErrInvalidType
	}
	content := strings.TrimSpace(f.Content)
	if len([]rune(content)) < 10 || len([]rune(content)) > 2000 {
		return ErrInvalidContent
	}
	if len([]rune(strings.TrimSpace(f.Contact))) > 100 {
		return ErrInvalidContact
	}
	return nil
}
