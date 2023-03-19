package notion_ical

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

var ErrParseDate = errors.New("date parsing error")

func parseNotionDateRange(r string, zone *time.Location) (time.Time, time.Time, error) {
	parts := strings.SplitN(r, "\u2192", 2)

	t1, err := parseNotionDate(parts[0], zone)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}

	if len(parts) == 2 {
		t2, err := parseNotionDate(parts[1], zone)
		if err != nil {
			t2, err = parseNotionTime(parts[1], zone)
			t2 = mergeNotionDateTime(t1, t2)
		}

		if err != nil {
			return time.Time{}, time.Time{}, err
		}

		return t1, t2, nil
	}

	return t1, t1, nil
}

var notionTimeFormats = []string{"15:04", "3:00 PM"}
var notionDateFormats = []string{"January 2, 2006", "2006/01/02"}

func parseNotionDate(d string, zone *time.Location) (time.Time, error) {
	var t time.Time
	var err error

	d = strings.TrimSpace(d)

	for _, fd := range notionDateFormats {
		for _, ft := range notionTimeFormats {
			f := fd + " " + ft
			t, err = time.ParseInLocation(f, d, zone)
			if err == nil {
				return t, nil
			}
		}
	}

	return t, fmt.Errorf("%w: %s is not a valid date", ErrParseDate, d)
}

func parseNotionTime(d string, zone *time.Location) (time.Time, error) {
	var t time.Time
	var err error

	d = strings.TrimSpace(d)

	for _, f := range notionTimeFormats {
		t, err = time.ParseInLocation(f, d, zone)
		if err == nil {
			return t, nil
		}
	}

	return t, fmt.Errorf("%w: %s is not a valid date", ErrParseDate, d)
}

func mergeNotionDateTime(date time.Time, t time.Time) time.Time {
	return time.Date(date.Year(), date.Month(), date.Day(), t.Hour(), t.Minute(), t.Second(), 0, t.Location())
}
