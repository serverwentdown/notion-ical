package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/serverwentdown/notion-ical"
	"github.com/urfave/cli/v2"
)

const (
	SourceTypeExport = "export"
	SourceTypeAPI    = "api"
)

func main() {
	app := &cli.App{
		Name:                 "notion-ical",
		Usage:                "generate iCal events from a Notion export or the Notion API",
		EnableBashCompletion: true,
		Suggest:              true,
		Flags: []cli.Flag{
			&cli.PathFlag{
				Name:    "export",
				Aliases: []string{"e"},
				Usage:   "read events from this export ZIP file",
			},
			&cli.StringFlag{
				Name:    "export-timezone",
				Aliases: []string{"z"},
				Usage:   "timezone to interpret dates in the export",
				Value:   "Local",
			},
			&cli.StringFlag{
				Name:    "api-key",
				Aliases: []string{"k"},
				EnvVars: []string{"NOTION_API_KEY"},
				Usage:   "read events from the API using this API key",
			},
			&cli.StringFlag{
				Name:    "database-id",
				Aliases: []string{"d"},
				EnvVars: []string{"NOTION_DATABASE_ID"},
				Usage:   "read events from this database ID",
			},
			&cli.StringFlag{
				Name:    "date-property",
				EnvVars: []string{"NOTION_DATE_PROPERTY"},
				Usage:   "use this date property for the event date instead of looking for the first date property",
			},
			&cli.StringFlag{
				Name:    "hide-property",
				EnvVars: []string{"NOTION_HIDE_PROPERTY"},
				Usage:   "hide events that have this checkbox property set",
			},
		},
		Commands: []*cli.Command{
			{
				Name:  "save",
				Usage: "save iCal events to a file",
				Flags: []cli.Flag{
					&cli.PathFlag{
						Name:     "output",
						Aliases:  []string{"o"},
						Usage:    "output iCal file path",
						Required: true,
					},
				},
				Action: func(ctx *cli.Context) error {
					source, err := sourceFromFlags(ctx)
					if err != nil {
						return err
					}

					f, err := os.Create(ctx.String("output"))
					if err != nil {
						return fmt.Errorf("unable to open output file: %w", err)
					}
					defer f.Close()

					return notion_ical.Convert(source, f)
				},
			},
			{
				Name:  "serve",
				Usage: "serve iCal over HTTP",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:    "listen",
						Aliases: []string{"l"},
						Usage:   "host and port to listen on",
						Value:   ":8080",
					},
					&cli.DurationFlag{
						Name:    "cache",
						Aliases: []string{"c"},
						Usage:   "cache duration to limit request rate to Notion API",
						Value:   30 * time.Second,
					},
				},
				Action: func(ctx *cli.Context) error {
					return nil
				},
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}

func sourceFromFlags(ctx *cli.Context) (notion_ical.Source, error) {
	if ctx.String("export") != "" && ctx.String("api-key") != "" {
		err := cli.ShowAppHelp(ctx)
		if err != nil {
			log.Fatal(err)
		}
		return nil, fmt.Errorf("Either \"export\" or \"api-key\" should be set")
	}
	if ctx.String("export") != "" {
		archive, err := os.Open(ctx.Path("export"))
		if err != nil {
			return nil, fmt.Errorf("error opening archive: %w", err)
		}

		zone, err := time.LoadLocation(ctx.String("export-timezone"))
		if err != nil {
			return nil, fmt.Errorf("error loading timezone: %w", err)
		}

		return notion_ical.NewSourceExport(notion_ical.ConfigSourceExport{
			Archive:      archive,
			Zone:         zone,
			DateProperty: ctx.String("date-property"),
			HideProperty: ctx.String("hide-property"),
		})
	} else if ctx.String("api-key") != "" {
		if ctx.String("database-id") == "" {
			err := cli.ShowAppHelp(ctx)
			if err != nil {
				log.Fatal(err)
			}
			return nil, fmt.Errorf("Required flag \"database-id\" not set")
		}
		return notion_ical.NewSourceAPI(notion_ical.ConfigSourceAPI{
			APIKey:       ctx.String("api-key"),
			DatabaseID:   ctx.String("database-id"),
			DateProperty: ctx.String("date-property"),
			HideProperty: ctx.String("hide-property"),
		})
	} else {
		err := cli.ShowAppHelp(ctx)
		if err != nil {
			log.Fatal(err)
		}
		return nil, fmt.Errorf("One of \"export\" or \"api-key\" should be set")
	}
}
