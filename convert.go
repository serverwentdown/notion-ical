package notion_ical

import (
	"io"
	"log"

	"github.com/arran4/golang-ical"
)

func Convert(source Source, ical io.Writer) error {
	events, err := source.ReadAll()
	if err != nil {
		return err
	}

	// Create calendar
	cal := ics.NewCalendar()
	// Set calendar properties
	cal.SetName(source.Name())
	cal.SetProductId("-//Ambrose Chua//serverwentdown notion-ical//EN")
	cal.SetRefreshInterval("P12H")

	// Add events to calendar
	for _, event := range events {
		calEvent := cal.AddEvent(event.ID)
		calEvent.SetSummary(event.Title)
		calEvent.SetDtStampTime(event.Start)
		calEvent.SetStartAt(event.Start)
		calEvent.SetEndAt(event.End)
		calEvent.SetDescription(event.Description())
	}

	log.Printf("Processed %d events", len(events))

	return cal.SerializeTo(ical)
}
