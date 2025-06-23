package client

import (
	"fmt"
	"os"
	"strings"

	"github.com/olekukonko/tablewriter"
	"github.com/olekukonko/tablewriter/tw"
	"golang.org/x/term"
)

func RenderTableString(headers []string, data [][]string, conf tablewriter.Config) string {
	config := tablewriter.WithConfig(conf)

	tableString := &strings.Builder{}
	table := tablewriter.NewTable(tableString, config)
	table.Header(headers)
	table.Bulk(data)
	table.Render()

	return tableString.String()
}

// RenderTable renders a table to stdout
// Note: tablewriter v1.0.7: can't get consistent results with MaxWidth,
// so we use an internal table hack
func RenderTable(headers []string, data [][]string) {
	// compute screen overflow
	width, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		width = 0
	}

	conf := tablewriter.Config{}

	internalTableString := RenderTableString(headers, data, conf)

	// read first line to get the width of the table
	firstLine := strings.Split(internalTableString, "\n")[0]
	firstLineLen := len([]rune(firstLine))
	overflow := firstLineLen - width

	largestColWidth := 0
	currentColWidth := 0

	// (anything else than '─' is a column separator)
	for _, r := range firstLine {
		if r != '─' {
			if currentColWidth > largestColWidth {
				largestColWidth = currentColWidth
			}
			currentColWidth = 0
		} else {
			currentColWidth++
		}
	}

	newMaxColWidth := largestColWidth - overflow
	if newMaxColWidth < 10 {
		newMaxColWidth = 10
	}

	// is truncation needed?
	conf.Row = tw.CellConfig{
		ColMaxWidths: tw.CellWidth{Global: newMaxColWidth},
	}

	internalTableString = RenderTableString(headers, data, conf)
	firstLine = strings.Split(internalTableString, "\n")[0]
	firstLineLen = len([]rune(firstLine))
	if firstLineLen <= width {
		// no
		fmt.Print(internalTableString)
		return
	}

	// yes, we need to truncate
	conf.Row.Formatting = tw.CellFormatting{
		AutoWrap: tw.WrapTruncate,
	}

	tableString := RenderTableString(headers, data, conf)
	fmt.Print(tableString)
}
