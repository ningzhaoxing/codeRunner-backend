package prompt

import (
	"strings"
)

type Section struct {
	Name    string
	Stable  bool
	Content string
}

type Sections []Section

func (s Sections) Render() string {
	var sb strings.Builder
	for _, sec := range s {
		if strings.TrimSpace(sec.Content) == "" {
			continue
		}
		sb.WriteString(sec.Content)
		sb.WriteString("\n\n")
	}
	return strings.TrimSpace(sb.String())
}
