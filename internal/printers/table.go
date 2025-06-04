package printers

import (
	"io"

	"github.com/olekukonko/tablewriter"
	"github.com/olekukonko/tablewriter/renderer"
	"github.com/olekukonko/tablewriter/tw"
)

// PrintTable writes an terminal-friendly table of the values to the target.
func PrintTable(target io.Writer, headers []string, rows [][]string) {
	table := tablewriter.NewTable(target,
		tablewriter.WithRenderer(renderer.NewBlueprint()),
		tablewriter.WithRowAutoWrap(tw.WrapNone),
		tablewriter.WithHeaderAutoFormat(tw.On),
		tablewriter.WithHeaderAlignment(tw.AlignLeft),
		tablewriter.WithRowAlignment(tw.AlignLeft),
		tablewriter.WithRendition(tw.Rendition{
			Symbols: tw.NewSymbolCustom("custom").WithCenter("").WithColumn("").WithRow(""),
			Settings: tw.Settings{
				Lines: tw.LinesNone,
				Separators: tw.Separators{
					BetweenColumns: tw.On,
				},
			},
			Borders: tw.BorderNone,
		}),
		tablewriter.WithTrimSpace(tw.Off),
	)
	table.Header(headers)
	_ = table.Bulk(rows)
	_ = table.Render()
}
