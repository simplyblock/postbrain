package ui

import (
	"fmt"
	"time"
)

// truncate returns s truncated to n bytes with an ellipsis appended if truncated.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// timeAgo returns a human-readable relative time string.
func timeAgo(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}

// derefString dereferences a *string, returning "" if nil.
func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// derefTime dereferences a *time.Time, returning "" if nil, else RFC3339 string.
func derefTime(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format(time.RFC3339)
}

// statusBadge maps a knowledge artifact status to a CSS badge class.
func statusBadge(status string) string {
	switch status {
	case "published":
		return "badge-ok"
	case "in_review":
		return "badge-warn"
	case "deprecated":
		return "badge-err"
	default:
		return ""
	}
}
