package host

import (
	"fmt"
	"io"
	"strconv"
	"strings"
)

// PortCheckSide controls which probe sides are run.
type PortCheckSide int

const (
	CheckSideInternet  PortCheckSide = 1 << 0
	CheckSideLocalhost PortCheckSide = 1 << 1
	CheckSideBoth      PortCheckSide = CheckSideInternet | CheckSideLocalhost
	CheckSideHost      PortCheckSide = 1 << 2
)

// PortSideResult holds the outcome of one side (internet or localhost) of a port check.
type PortSideResult struct {
	Status   string // OPEN, CLOSED, LISTENING, UNKNOWN, ERROR
	Provider string
}

// PortRowResult holds both sides of a port check for one target.
type PortRowResult struct {
	Target    PortCheckTarget
	Internet  PortSideResult
	Localhost PortSideResult
}

const (
	PortStatusOpen      = "OPEN"
	PortStatusClosed    = "CLOSED"
	PortStatusListening = "LISTENING"
	PortStatusUnknown   = "UNKNOWN"
	PortStatusError     = "ERROR"
	PortStatusSkipped   = "-"
)

const (
	ansiGreen  = "\033[32m"
	ansiRed    = "\033[31m"
	ansiYellow = "\033[33m"
	ansiReset  = "\033[0m"
)

// ColorizePortStatus wraps status strings with ANSI colour codes.
func ColorizePortStatus(status string) string {
	switch status {
	case PortStatusOpen:
		return ansiGreen + status + ansiReset
	case PortStatusClosed, PortStatusError:
		return ansiRed + status + ansiReset
	case PortStatusUnknown, PortStatusListening:
		return ansiYellow + status + ansiReset
	default:
		return status
	}
}

// ToInternetPortStatus maps a raw provider response to a canonical status.
func ToInternetPortStatus(isOpen bool, raw string) string {
	lower := strings.ToLower(strings.TrimSpace(raw))
	if isOpen || lower == "open" || strings.Contains(lower, "open") || lower == "true" {
		return PortStatusOpen
	}
	if strings.Contains(lower, "closed") || strings.Contains(lower, "reject") || lower == "false" {
		return PortStatusClosed
	}
	return PortStatusUnknown
}

// ToLocalhostPortStatus maps a raw localhost check result to a canonical status.
func ToLocalhostPortStatus(isOpen bool, raw string) string {
	lower := strings.ToLower(strings.TrimSpace(raw))
	if isOpen || lower == "open" {
		return PortStatusOpen
	}
	if lower == "listening" {
		return PortStatusListening
	}
	if strings.Contains(lower, "closed") {
		return PortStatusClosed
	}
	return PortStatusUnknown
}

// PrintPortCheckTable renders results as a table to w.  The columns shown
// depend on which sides were probed.  publicIP is included in the INTERNET
// column header; host is included in the HOST column header.
func PrintPortCheckTable(w io.Writer, publicIP string, host string, side PortCheckSide, rows []PortRowResult) {
	header := []string{"LABEL", "PORT", "PROTO"}
	if side&CheckSideInternet != 0 {
		col := "INTERNET"
		if publicIP != "" {
			col = fmt.Sprintf("INTERNET (%s)", publicIP)
		}
		header = append(header, col)
	}
	if side&CheckSideLocalhost != 0 {
		header = append(header, "LOCALHOST")
	}
	if side&CheckSideHost != 0 {
		col := "HOST"
		if host != "" {
			col = fmt.Sprintf("HOST (%s)", host)
		}
		header = append(header, col)
	}

	tableRows := [][]string{header}
	hasUDPUnknownInternet := false
	hasListening := false

	for _, r := range rows {
		row := []string{r.Target.Label, strconv.Itoa(r.Target.HostPort), r.Target.Protocol}
		if side&CheckSideInternet != 0 {
			row = append(row, ColorizePortStatus(r.Internet.Status))
			if r.Target.Protocol == ProtocolUDP && r.Internet.Status == PortStatusUnknown {
				hasUDPUnknownInternet = true
			}
		}
		if side&CheckSideLocalhost != 0 {
			row = append(row, ColorizePortStatus(r.Localhost.Status))
			if r.Localhost.Status == PortStatusListening {
				hasListening = true
			}
		}
		if side&CheckSideHost != 0 {
			row = append(row, ColorizePortStatus(r.Localhost.Status))
			if r.Localhost.Status == PortStatusListening {
				hasListening = true
			}
		}
		tableRows = append(tableRows, row)
	}

	formatTable(w, tableRows)

	if hasUDPUnknownInternet {
		fmt.Fprintln(w, "Note: UDP UNKNOWN means the external checker received no response. ARK does not reply to port probes.")
		fmt.Fprintln(w, "      Use --listener with the server stopped for a definitive UDP forwarding test.")
	}
	if hasListening {
		fmt.Fprintln(w, "Note: LISTENING means the port is bound but the process did not respond to a probe.")
		fmt.Fprintln(w, "      This is normal for ARK's UDP game port.")
	}
}

// PrintPortPreCheckWarning prints a hint when any localhost port appears closed.
func PrintPortPreCheckWarning(w io.Writer, rows []PortRowResult) {
	for _, r := range rows {
		if r.Localhost.Status == PortStatusClosed {
			fmt.Fprintln(w, "Warning: one or more ports have nothing listening locally.")
			fmt.Fprintln(w, "         If the server is not running, consider --listener to test port forwarding without a live server.")
			return
		}
	}
}
