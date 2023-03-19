package notion_ical

import (
	"errors"
)

var ErrNoDateProperty = errors.New("no date property")
var ErrNoHideProperty = errors.New("no hide property")
var ErrNoTitleProperty = errors.New("no title property")

type Source interface {
	Name() string
	ReadAll() ([]Event, error)
}
