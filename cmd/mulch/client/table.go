package client

import (
	"fmt"
	"os"
	"strings"

	"github.com/olekukonko/tablewriter"
	"golang.org/x/term"
)

// RenderStringTable renders a table as a string
func RenderStringTable(headers []string, data [][]string, tableCallback func(*tablewriter.Table)) string {
	tableString := &strings.Builder{}
	table := tablewriter.NewWriter(tableString)
	table.SetHeader(headers)
	table.SetBorders(tablewriter.Border{Left: true, Top: false, Right: true, Bottom: false})
	table.SetCenterSeparator("|")
	// apply callback if provided
	if tableCallback != nil {
		tableCallback(table)
	}
	table.AppendBulk(data)
	table.Render()

	return tableString.String()
}

// RenderTable renders a table to stdout
func RenderTable(headers []string, data [][]string, tableCallback func(*tablewriter.Table)) {
	fmt.Print(RenderStringTable(headers, data, tableCallback))
}

// RenderTableTruncateCol renders a table to stdout, truncatting the column
// colNum if table does not fit the screen width
func RenderTableTruncateCol(colNum int, headers []string, data [][]string, tableCallback func(*tablewriter.Table)) {
	tableStr := RenderStringTable(headers, data, tableCallback)

	// find the longest line of tableString
	lines := strings.Split(tableStr, "\n")
	longestLine := len(lines[0]) // all lines have the same length

	// compute screen overflow
	width, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		width = 0
	}

	overflow := longestLine - width
	if width == 0 {
		overflow = 0
	}

	// get longest value for colNum column
	longestName := 0
	for _, line := range data {
		l := len(line[colNum])
		if l > longestName {
			longestName = l
		}
	}

	maxLen := longestName - overflow
	if maxLen < 5 {
		// don't shorten names if we only have a few characters, it will be unreadable
		maxLen = longestName
	}

	for _, line := range data {
		name := line[colNum]
		if len(name) > maxLen {
			line[colNum] = name[:maxLen-1] + "…"
		}
	}

	RenderTable(headers, data, tableCallback)
}
