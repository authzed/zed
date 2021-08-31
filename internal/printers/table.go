package printers

import (
	"io"

	"github.com/olekukonko/tablewriter"
)

// PrintTable writes an terminal-friendly table of the values to the target.
func PrintTable(target io.Writer, headers []string, rows [][]string) {
	table := tablewriter.NewWriter(target)
	table.SetHeader(headers)
	table.SetAutoWrapText(false)
	table.SetAutoFormatHeaders(true)
	table.SetHeaderAlignment(tablewriter.ALIGN_LEFT)
	table.SetAlignment(tablewriter.ALIGN_LEFT)
	table.SetCenterSeparator("")
	table.SetColumnSeparator("")
	table.SetRowSeparator("")
	table.SetHeaderLine(false)
	table.SetBorder(false)
	table.SetTablePadding("\t")
	table.SetNoWhiteSpace(true)
	table.AppendBulk(rows)
	table.Render()
}
