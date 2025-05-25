package dto

import (
	"fmt"
	"strings"
)

type Severity int64

// String returns the string representation of the Severity.
func (sev Severity) String() string {
	switch sev {
	case Critical:
		return "Critical"
	case Major:
		return "Major"
	case Minor:
		return "Minor"
	default:
		return "Unknown"
	}
}

// ParseSeverity parses a string and returns the corresponding Severity.
func ParseSeverity(sev string) (Severity, error) {
	switch strings.ToLower(sev) {
	case "critical":
		return Critical, nil
	case "major":
		return Major, nil
	case "minor":
		return Minor, nil
	default:
		return -1, fmt.Errorf("invalid severity: [%s]", sev)
	}
}

// Severity levels for the host status
const (
	Critical Severity = iota
	Major    Severity = iota
	Minor    Severity = iota
)

type Host struct {
	Host              string
	Timeout           int
	Interval          int
	Headers           map[string]string
	Hooks             []string
	AppGroup          []string
	Severity          Severity
	OutageDescription string
}
