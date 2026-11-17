package core

import (
	"regexp"
	"strings"
)

var ansiEscapeRe = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// VisualLen returns the visual length of a string, excluding ANSI escape codes.
func VisualLen(s string) int {
	return len(ansiEscapeRe.ReplaceAllString(s, ""))
}

// PrintTable formats rows as a space-aligned table (2-space column padding) and
// prints the result via Infof. The first row is conventionally the header.
// Column widths are computed from visual length so ANSI colour codes do not
// disturb alignment.
func PrintTable(rows [][]string) {
	if len(rows) == 0 {
		return
	}
	cols := 0
	for _, row := range rows {
		if len(row) > cols {
			cols = len(row)
		}
	}
	widths := make([]int, cols)
	for _, row := range rows {
		for j, cell := range row {
			if w := VisualLen(cell); w > widths[j] {
				widths[j] = w
			}
		}
	}
	var sb strings.Builder
	for _, row := range rows {
		for j, cell := range row {
			sb.WriteString(cell)
			if j < len(row)-1 {
				sb.WriteString(strings.Repeat(" ", widths[j]-VisualLen(cell)+2))
			}
		}
		sb.WriteByte('\n')
	}
	Infof("%s", sb.String())
}
