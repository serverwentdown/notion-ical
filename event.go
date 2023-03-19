package notion_ical

import (
	"strings"
	"time"
)

type Event struct {
	ID    string
	Title string
	Emoji string
	URL   string

	Start time.Time
	End   time.Time

	Content []string
	Properties []EventProperty
}

func (e Event) Description() string {
	var s []string
	for _, property := range e.Properties {
		line := property.NameString() + ":"

		value := property.ValueString()
		if strings.Contains(value, "\n") {
			line += "\n" + value
		} else {
			line += " " + value
		}

		s = append(s, line, "\n")
	}

	for _, content := range e.Content {
		s = append(s, content, "\n\n")
	}

	return strings.Join(s, "")
}

type EventProperty interface {
	NameString() string
	ValueString() string
}
