package core

import "strings"

// PrintDetailKV prints aligned "key: value" rows via Infof. Keys are
// left-padded so values all start at the same column. Alignment matches
// the convention used today by `show instance`, `show backup`,
// `schedule show`, and `health`: the column where values start equals
// the widest key's length + 3 (one for the colon, two for minimum
// separator).
//
// For a row with key "session" and value "MyServer", and another with
// key "port" and value "27015", output is:
//
//	session:  MyServer
//	port:     27015
//
// The input slice is a list of [key, value] pairs so callers can build
// the block procedurally without needing a map (order must be stable).
// Empty input prints nothing.
func PrintDetailKV(rows [][2]string) {
	if len(rows) == 0 {
		return
	}
	maxKey := 0
	for _, row := range rows {
		if w := VisualLen(row[0]); w > maxKey {
			maxKey = w
		}
	}
	var sb strings.Builder
	for _, row := range rows {
		sb.WriteString(row[0])
		sb.WriteByte(':')
		sb.WriteString(strings.Repeat(" ", maxKey+2-VisualLen(row[0])))
		sb.WriteString(row[1])
		sb.WriteByte('\n')
	}
	Infof("%s", sb.String())
}

// PrintPipeTable prints a pipe-separated table with a horizontal rule
// between the header and data rows. The first row is treated as the
// header. Column widths are computed from the widest cell (header or
// data) so the output stays aligned regardless of content length.
//
// Example output for header ["instance", "action"] and one data row
// ["asa1", "update"]:
//
//	instance | action
//	---------|-------
//	asa1     | update
//
// Used for machine-parseable pipe tables (`update check`, similar).
// Empty input or a single-row input (header only) prints nothing —
// producing a divider alone would be misleading.
func PrintPipeTable(rows [][]string) {
	if len(rows) < 2 {
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
	writeRow := func(row []string) {
		for j := range cols {
			if j > 0 {
				sb.WriteString(" | ")
			}
			cell := ""
			if j < len(row) {
				cell = row[j]
			}
			sb.WriteString(cell)
			sb.WriteString(strings.Repeat(" ", widths[j]-VisualLen(cell)))
		}
		sb.WriteByte('\n')
	}
	writeRow(rows[0])
	for j := range cols {
		if j > 0 {
			sb.WriteString("-|-")
		}
		sb.WriteString(strings.Repeat("-", widths[j]))
	}
	sb.WriteByte('\n')
	for _, row := range rows[1:] {
		writeRow(row)
	}
	Infof("%s", sb.String())
}
