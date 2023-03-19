package notion_ical

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"strings"
	"time"
)

var ErrCSVRead = errors.New("failed to read CSV")

type ReaderAtSeeker interface {
	io.ReaderAt
	io.Seeker
}

// ConfigSourceExport represents configuration for importing from a Notion export.
type ConfigSourceExport struct {
	// Archive is a file handle to a ZIP file of the exported Notion data.
	Archive ReaderAtSeeker
	// Zone is the timezone for parsing dates.
	Zone *time.Location
	// DateProperty is the property name of the date field that will be used
	// as the event date.
	DateProperty string
	// HideProperty is the property name of a checkbox that will cause
	// events to be hidden.
	HideProperty string
}

type SourceExport struct {
	config  ConfigSourceExport
	archive fs.FS
	name    string
}

func NewSourceExport(config ConfigSourceExport) (SourceExport, error) {
	// Find the length of the archive
	length, err := config.Archive.Seek(0, io.SeekEnd)
	if err != nil {
		return SourceExport{}, fmt.Errorf("unable to obtain file size: %w", err)
	}

	// Open the ZIP file
	archive, err := zip.NewReader(config.Archive, length)
	if err != nil {
		return SourceExport{}, fmt.Errorf("unable to open ZIP file: %w", err)
	}

	var name string

	// Find the first CSV file
	for _, file := range archive.File {
		if strings.HasSuffix(file.Name, ".csv") {
			name = file.Name
		}
	}

	if name == "" {
		return SourceExport{}, fmt.Errorf("cannot find CSV file in ZIP file")
	}

	return SourceExport{
		config:  config,
		archive: archive,
		name:    name,
	}, nil
}

func (s SourceExport) Name() string {
	return s.name
}

func (s SourceExport) ReadAll() ([]Event, error) {
	// Open CSV file
	f, err := s.archive.Open(s.name)
	if err != nil {
		return nil, fmt.Errorf("%w: failed open: %w", ErrCSVRead, err)
	}
	defer f.Close()

	// Open CSV reader
	csvReader := csv.NewReader(f)

	// Read the first row as headers
	headers, err := csvReader.Read()
	if err != nil {
		return nil, fmt.Errorf("%w: headers: %v", ErrCSVRead, err)
	}

	events := make([]Event, 0)

	for {
		// Read one row
		record, err := csvReader.Read()
		if errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrCSVRead, err)
		}

		// Convert it to an event
		event, err := s.eventFromCSVRow(headers, record)
		if err != nil {
			return nil, err
		}

		events = append(events, event)
	}

	return events, nil
}

func (s SourceExport) eventFromCSVRow(headers []string, record []string) (Event, error) {
	m, err := headersAndRecordToMap(headers, record)
	if err != nil {
		return Event{}, err
	}

	var dateKey, date string
	if s.config.DateProperty == "" {
		// Find first date column
		dateKey, date = findFirstColumn([]string{"date", "when", "period"}, m)
		if dateKey == "" {
			return Event{}, ErrNoDateProperty
		}
	} else {
		dateKey = s.config.DateProperty
		var ok bool
		date, ok = m[dateKey]
		if !ok {
			return Event{}, ErrNoDateProperty
		}
	}

	// Parse date range
	start, end, err := parseNotionDateRange(date, s.config.Zone)
	if err != nil {
		return Event{}, err
	}

	// Find first title column
	titleKey, title := findFirstColumn([]string{"name", "title"}, m)
	if titleKey == "" {
		return Event{}, ErrNoTitleProperty
	}

	properties := []EventProperty{}

	// Generate properties list
	for i, key := range headers {
		if key == dateKey || key == titleKey {
			continue
		}
		value := record[i]
		property := exportProperty{key, value}
		properties = append(properties, property)
	}

	// Generate a ID based on the title and date
	titleBytes := []byte(title)
	dateBytes, err := start.MarshalText()
	if err != nil {
		return Event{}, err
	}
	idBytes := append(titleBytes, dateBytes...)
	titleHash := sha256.Sum256(idBytes)
	titleHashHex := hex.EncodeToString(titleHash[:])
	id := titleHashHex + "@notion-ical-export"

	return Event{
		ID:         id,
		Title:      title,
		Start:      start,
		End:        end,
		Properties: properties,
	}, nil
}

type exportProperty struct {
	name  string
	value string
}

func (p exportProperty) NameString() string {
	return p.name
}

func (p exportProperty) ValueString() string {
	return p.value
}

func headersAndRecordToMap(headers []string, record []string) (map[string]string, error) {
	m := make(map[string]string)

	if len(headers) != len(record) {
		return nil, fmt.Errorf("%w: unmatching header and record length", ErrCSVRead)
	}

	for i, value := range record {
		key := headers[i]
		m[key] = value
	}

	return m, nil
}

func findFirstColumn(names []string, m map[string]string) (string, string) {
	for key, value := range m {
		keyLower := strings.ToLower(key)
		for _, q := range names {
			qLower := strings.ToLower(q)
			if keyLower == qLower {
				return key, value
			}
		}
	}

	for key, value := range m {
		keyLower := strings.ToLower(key)
		for _, q := range names {
			qLower := strings.ToLower(q)
			if strings.Contains(keyLower, qLower) {
				return key, value
			}
		}
	}

	return "", ""
}
