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
		return "critical"
	case Major:
		return "major"
	case Minor:
		return "minor"
	default:
		return "unknown"
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
	// Critical implies a severe issue that requires immediate attention.
	Critical Severity = iota
	// Major implies a significant issue that needs attention, but allows the user to continue operations.
	Major Severity = iota
	// Minor is the lowest severity level, indicating a minor issue.
	Minor Severity = iota
)

type Host struct {
	// Host is the URL of the host to be monitored.
	Host string
	// Host is the URL of the host to be monitored.
	Timeout int
	// Interval is the time in seconds between each check.
	Interval int
	// Headers is a map of headers to be sent with each request.
	Headers map[string]string
	// Hooks is a list of hooks to be executed when the host status changes.
	Hooks []string
	// AppGroup is a list of application groups this host belongs to.
	AppGroup []string
	// Severity indicates the severity level of the host.
	Severity Severity
	// OutageDescription is a description of the outage, if any.
	OutageDescription string
	// OutageDownThreshold is the number of consecutive down checks before hooks are triggered.
	OutageDownThreshold int
}
