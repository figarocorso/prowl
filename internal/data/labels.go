package data

import (
	"fmt"
	"strings"
)

// StatusLabel returns the human-friendly status column ("open", "draft",
// "open/blocked", "merged", "closed", "unknown").
func StatusLabel(pr PR) string {
	switch strings.ToUpper(pr.State) {
	case "MERGED":
		return "merged"
	case "CLOSED":
		return "closed"
	case "OPEN":
		if pr.IsDraft {
			return "draft"
		}
		switch strings.ToUpper(pr.MergeStateStatus) {
		case "BLOCKED", "BEHIND", "DIRTY", "DRAFT":
			return "open/blocked"
		default:
			return "open"
		}
	case "":
		return "unknown"
	default:
		return strings.ToLower(pr.State)
	}
}

// QueueLabel returns the merge-queue column ("-" when not queued).
func QueueLabel(pr PR) string {
	if pr.Queue == nil {
		return "-"
	}
	switch strings.ToUpper(pr.Queue.State) {
	case "":
		return "-"
	case "AWAITING_CHECKS":
		return "queued (awaiting checks)"
	case "MERGEABLE":
		return "queued (mergeable)"
	case "LOCKED":
		return "queued (locked)"
	case "UNMERGEABLE":
		return "queued (unmergeable)"
	case "QUEUED":
		return "queued"
	default:
		return "queued (" + strings.ToLower(strings.ReplaceAll(pr.Queue.State, "_", " ")) + ")"
	}
}

// QueueLabelShort returns a compact (<= 8 chars) queue state for table display.
// QueueLabel keeps the verbose form for single-PR views like `prowl get`.
func QueueLabelShort(pr PR) string {
	if pr.Queue == nil {
		return "-"
	}
	switch strings.ToUpper(pr.Queue.State) {
	case "":
		return "-"
	case "AWAITING_CHECKS":
		return "checks"
	case "MERGEABLE":
		return "ready"
	case "LOCKED":
		return "locked"
	case "UNMERGEABLE":
		return "blocked"
	case "QUEUED":
		return "queued"
	default:
		return strings.ToLower(pr.Queue.State)
	}
}

// ShortURL trims the `https://github.com/` (or www / http) prefix so URLs
// fit in a compact table column. Non-github URLs are returned unchanged.
func ShortURL(u string) string {
	for _, p := range []string{
		"https://www.github.com/",
		"https://github.com/",
		"http://www.github.com/",
		"http://github.com/",
	} {
		if strings.HasPrefix(u, p) {
			return u[len(p):]
		}
	}
	return u
}

// QueuePositionLabel renders the position column.
func QueuePositionLabel(pr PR) string {
	if pr.Queue == nil || pr.Queue.Position <= 0 {
		return "-"
	}
	return fmt.Sprintf("%d", pr.Queue.Position)
}

// ETALabel formats the queue ETA as ~Ns / ~Nm / ~Nh, "-" if unset.
func ETALabel(pr PR) string {
	if pr.Queue == nil || pr.Queue.ETA == 0 {
		return "-"
	}
	secs := int(pr.Queue.ETA.Seconds())
	switch {
	case secs < 60:
		return fmt.Sprintf("~%ds", secs)
	case secs < 3600:
		return fmt.Sprintf("~%dm", (secs+30)/60)
	default:
		return fmt.Sprintf("~%dh", (secs+1800)/3600)
	}
}

// AssigneesLabel renders the assignee column ("-" when empty).
func AssigneesLabel(pr PR) string {
	if len(pr.Assignees) == 0 {
		return "-"
	}
	return strings.Join(pr.Assignees, ",")
}

// IsTerminal reports whether the PR has reached a terminal state (merged or
// closed) and should be archived.
func IsTerminal(pr PR) bool {
	s := strings.ToUpper(pr.State)
	return s == "MERGED" || s == "CLOSED"
}
