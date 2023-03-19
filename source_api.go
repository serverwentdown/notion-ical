package notion_ical

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/dstotijn/go-notion"
)

var ErrPropertyNotFound = errors.New("property not found in database")

// ConfigSourceAPI represents configuration for importing from the Notion API.
type ConfigSourceAPI struct {
	// APIKey is the Notion API key to use.
	APIKey string
	// DatabaseID is the database ID to get events from.
	DatabaseID string
	// DateProperty is the property name of the date field that will be used
	// as the event date.
	DateProperty string
	// HideProperty is the property name of a checkbox that will cause
	// events to be hidden.
	HideProperty string
}

type SourceAPI struct {
	config   ConfigSourceAPI
	client   *notion.Client
	database notion.Database
}

func NewSourceAPI(config ConfigSourceAPI) (SourceAPI, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client := notion.NewClient(config.APIKey)

	// Checks that the database exists, and also fetches the database name
	database, err := client.FindDatabaseByID(ctx, config.DatabaseID)
	if err != nil {
		return SourceAPI{}, err
	}

	// Check that DateProperty and HideProperty exists
	datePropertyMatches := 0
	hidePropertyMatches := 0
	var propertyNames []string

	// Loop through each property and find any matching ones
	for name, property := range database.Properties {
		propertyNames = append(propertyNames, name)
		switch property.Type {
		case "date":
			if config.DateProperty == "" {
				datePropertyMatches += 1
			} else if name == config.DateProperty {
				datePropertyMatches += 1
			}
		case "checkbox":
			if config.HideProperty == "" {
				continue
			} else if name == config.HideProperty {
				hidePropertyMatches += 1
			}
		}
	}

	if datePropertyMatches != 1 {
		return SourceAPI{}, fmt.Errorf("%w: %s not in %v", ErrNoDateProperty, config.DateProperty, propertyNames)
	}
	if config.HideProperty != "" && hidePropertyMatches != 1 {
		return SourceAPI{}, fmt.Errorf("%w: %s not in %v", ErrNoHideProperty, config.DateProperty, propertyNames)
	}

	// Titles are guaranteed to exist

	return SourceAPI{
		config:   config,
		client:   client,
		database: database,
	}, nil
}

func (s SourceAPI) Name() string {
	return richTextToString(s.database.Title)
}

func (s SourceAPI) ReadAll() ([]Event, error) {
	events := make([]Event, 0)
	query := s.initialQuery()

	for {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		response, err := s.client.QueryDatabase(ctx, s.database.ID, query)
		cancel()
		if err != nil {
			return nil, err
		}

		for _, page := range response.Results {
			event, err := s.eventFromPage(page)
			if err != nil {
				return nil, err
			}

			events = append(events, event)
		}

		if !response.HasMore {
			break
		}
		query.StartCursor = *response.NextCursor
	}

	return events, nil
}

func (s SourceAPI) eventFromPage(page notion.Page) (Event, error) {
	var title, emoji string
	var start, end time.Time

	if page.Icon != nil && page.Icon.Emoji != nil {
		emoji = *page.Icon.Emoji
	}

	properties := page.Properties.(notion.DatabasePageProperties)
	var propertiesList []EventProperty

	// Loop through each property and find any matching ones
	for name, property := range properties {
		switch property.Type {
		case notion.DBPropTypeTitle:
			title = richTextToString(property.Title)
			continue
		case notion.DBPropTypeDate:
			if s.config.DateProperty == "" {
				start = property.Date.Start.Time
				end = property.Date.End.Time
				continue
			} else if name == s.config.DateProperty {
				start = property.Date.Start.Time
				end = property.Date.End.Time
				continue
			}
		case notion.DBPropTypeRelation:
			continue
		}
		// Because QueryDatabase does not populate Name, manually populate it
		if property.Name == "" {
			property.Name = name
		}
		propertiesList = append(propertiesList, apiProperty(property))
	}

	// Sort properties by name
	sort.Slice(propertiesList, func(i, j int) bool {
		return strings.Compare(propertiesList[i].NameString(), propertiesList[j].NameString()) < 0
	})

	// Get page content
	content, err := s.getPageContentPlain(page.ID)
	if err != nil {
		return Event{}, err
	}

	return Event{
		ID:         page.ID + "@notion-ical",
		Title:      title,
		Emoji:      emoji,
		URL:        page.URL,
		Start:      start,
		End:        end,
		Properties: propertiesList,
		Content:    content,
	}, nil
}

func (s SourceAPI) getPageContentPlain(id string) ([]string, error) {
	var content []string

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	block, err := s.client.FindBlockByID(ctx, id)
	cancel()
	if err != nil {
			return content, fmt.Errorf("failed fetching block %v: %w", id, err)
	}

	log.Printf("fetched block %v", id)

	switch b := block.(type) {
	case notion.ChildPageBlock:
		// Most page blocks should be this type
	default:
		content = append(content, s.convertBlockContentPlain(b))
	}

	if block.HasChildren() {
		childrenContent, err := s.getBlockChildrenContentPlain(id)
		if err != nil {
			return content, err
		}

		content = append(content, childrenContent...)
	}

	return content, nil
}

func (s SourceAPI) getBlockChildrenContentPlain(id string) ([]string, error) {
	var content []string

	query := &notion.PaginationQuery{
		StartCursor: "",
		PageSize:    100,
	}

	for {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		response, err := s.client.FindBlockChildrenByID(ctx, id, query)
		cancel()
		if err != nil {
			return content, fmt.Errorf("failed fetching child blocks for %v with query %#v: %w", id, query, err)
		}

		log.Printf("fetched child blocks for %v with query %#v and found %d child blocks", id, query, len(response.Results))

		for _, block := range response.Results {
			content = append(content, s.convertBlockContentPlain(block))

			if block.HasChildren() {
				childrenContent, err := s.getBlockChildrenContentPlain(block.ID())
				if err != nil {
					return content, err
				}

				content = append(content, childrenContent...)
			}
		}

		if !response.HasMore {
			break
		}
		query.StartCursor = *response.NextCursor
	}

	return content, nil
}

func (s SourceAPI) convertBlockContentPlain(block notion.Block) string {
	switch b := block.(type) {
	case *notion.ParagraphBlock:
		return richTextToString(b.RichText)
	case *notion.Heading1Block:
		return "# " + richTextToString(b.RichText)
	case *notion.Heading2Block:
		return "## " + richTextToString(b.RichText)
	case *notion.Heading3Block:
		return "### " + richTextToString(b.RichText)
	case *notion.BulletedListItemBlock:
		return "- " + richTextToString(b.RichText)
	case *notion.NumberedListItemBlock:
		return "* " + richTextToString(b.RichText)
	case *notion.ToDoBlock:
		prefix := "[ ] "
		if b.Checked != nil && *b.Checked == true {
			prefix = "[x] "
		}
		return prefix + richTextToString(b.RichText)
	case *notion.ToggleBlock:
		return "^ " + richTextToString(b.RichText)
	case *notion.CalloutBlock:
		return "! " + richTextToString(b.RichText)
	case *notion.QuoteBlock:
		return "> " + richTextToString(b.RichText)
	case *notion.CodeBlock:
		return "```\n" + richTextToString(b.RichText) + "\n```"
	case *notion.EmbedBlock:
		return "Embed: " + b.URL
	case *notion.ImageBlock:
		return "Image: " + fileToString(b.Type, b.File, b.External)
	case *notion.AudioBlock:
		return "Audio: " + fileToString(b.Type, b.File, b.External)
	case *notion.VideoBlock:
		return "Video: " + fileToString(b.Type, b.File, b.External)
	case *notion.FileBlock:
		return "File: " + fileToString(b.Type, b.File, b.External)
	case *notion.PDFBlock:
		return "PDF: " + fileToString(b.Type, b.File, b.External)
	case *notion.BookmarkBlock:
		return "Bookmark: " + b.URL
	case *notion.EquationBlock:
		return "Expression: " + b.Expression
	case *notion.DividerBlock:
		return "--------------------------"
	case *notion.TableOfContentsBlock:
		return ""
	case *notion.BreadcrumbBlock:
		return ""
	case *notion.ColumnListBlock:
		return ""
	case *notion.ColumnBlock:
		return ""
	case *notion.TableBlock:
		var st []string
		for _, block := range b.Children {
			st = append(st, s.convertBlockContentPlain(block))
		}
		return strings.Join(st, "\n")
	case *notion.TableRowBlock:
		var s []string
		for _, cell := range b.Cells {
			s = append(s, richTextToString(cell))
		}
		return strings.Join(s, ", ")
	case *notion.LinkPreviewBlock:
		return "Preview: " + b.URL
	case *notion.LinkToPageBlock:
		switch b.Type {
		case notion.LinkToPageTypePageID:
			return "Link: " + b.PageID
		case notion.LinkToPageTypeDatabaseID:
			return "Link: " + b.DatabaseID
		}
	case *notion.SyncedBlock:
		var sy []string
		for _, block := range b.Children {
			log.Printf("synced child block %v", block.ID())
			sy = append(sy, s.convertBlockContentPlain(block))
		}
		return strings.Join(sy, "\n\n")
	case *notion.TemplateBlock:
		return "Template: " + richTextToString(b.RichText)
	}
	return ""
}

func (s SourceAPI) initialQuery() *notion.DatabaseQuery {
	return &notion.DatabaseQuery{
		Filter:   s.filter(),
		PageSize: 100,
	}
}

var filterTrue = true

func (s SourceAPI) filter() *notion.DatabaseQueryFilter {
	if s.config.HideProperty == "" {
		return nil
	}
	return &notion.DatabaseQueryFilter{
		Property: s.config.HideProperty,
		DatabaseQueryPropertyFilter: notion.DatabaseQueryPropertyFilter{
			Checkbox: &notion.CheckboxDatabaseQueryFilter{
				DoesNotEqual: &filterTrue,
			},
		},
	}
}

type apiProperty notion.DatabasePageProperty

func (p apiProperty) NameString() string {
	return p.Name
}

func (p apiProperty) ValueString() string {
	switch p.Type {
	case notion.DBPropTypeTitle:
		return richTextToString(p.Title)
	case notion.DBPropTypeRichText:
		return richTextToString(p.RichText)
	case notion.DBPropTypeNumber:
		if p.Number != nil {
			return fmt.Sprintf("%f", *p.Number)
		}
	case notion.DBPropTypeSelect:
		if p.Select != nil {
			return p.Select.Name
		}
	case notion.DBPropTypeMultiSelect:
		var s []string
		for _, opt := range p.MultiSelect {
			s = append(s, opt.Name)
		}
		return strings.Join(s, ", ")
	case notion.DBPropTypeDate:
		if p.Date != nil {
			if p.Date.End != nil {
				return p.Date.Start.Format(time.DateTime) + " \u2192 " + p.Date.End.Format(time.DateTime)
			}
			return p.Date.Start.Format(time.DateTime)
		}
	case notion.DBPropTypeFormula:
		if p.Formula != nil {
			return fmt.Sprintf("%v", p.Formula.Value())
		}
	case notion.DBPropTypeRelation:
		var s []string
		for _, rel := range p.Relation {
			s = append(s, rel.ID)
		}
		return strings.Join(s, ", ")
	case notion.DBPropTypeRollup:
		if p.Rollup != nil {
			return fmt.Sprintf("%v", p.Rollup.Value())
		}
	case notion.DBPropTypePeople:
		var s []string
		for _, person := range p.People {
			s = append(s, person.Name)
		}
		return strings.Join(s, ", ")
	case notion.DBPropTypeFiles:
		var s []string
		for _, file := range p.Files {
			s = append(s, fileToString(file.Type, file.File, file.External))
		}
		return strings.Join(s, ", ")
	case notion.DBPropTypeCheckbox:
		if p.Checkbox != nil {
			if *p.Checkbox {
				return "Yes"
			} else {
				return "No"
			}
		}
	case notion.DBPropTypeURL:
		if p.URL != nil {
			return *p.URL
		}
	case notion.DBPropTypeEmail:
		if p.Email != nil {
			return *p.Email
		}
	case notion.DBPropTypePhoneNumber:
		if p.PhoneNumber != nil {
			return *p.PhoneNumber
		}
	case notion.DBPropTypeStatus:
		if p.Status != nil {
			return p.Status.Name
		}
	case notion.DBPropTypeCreatedTime:
		if p.CreatedTime != nil {
			return p.CreatedTime.Format(time.DateTime)
		}
	case notion.DBPropTypeCreatedBy:
		if p.CreatedBy != nil {
			return p.CreatedBy.Name
		}
	case notion.DBPropTypeLastEditedTime:
		if p.LastEditedTime != nil {
			return p.LastEditedTime.Format(time.DateTime)
		}
	case notion.DBPropTypeLastEditedBy:
		if p.LastEditedBy != nil {
			return p.LastEditedBy.Name
		}
	}
	return ""
}

func richTextToString(rt []notion.RichText) string {
	var s []string
	for _, rts := range rt {
		s = append(s, rts.PlainText)
	}

	return strings.Join(s, "")
}

func fileToString(t notion.FileType, f *notion.FileFile, e *notion.FileExternal) string {
	switch t {
	case notion.FileTypeFile:
		return f.URL
	case notion.FileTypeExternal:
		return e.URL
	}
	return ""
}
