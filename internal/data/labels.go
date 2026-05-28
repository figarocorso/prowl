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

// DetailsLabel returns the compact "status details" column. When the PR is in
// the merge queue it summarises queue state + position + ETA; when the PR is
// open/blocked it explains why (review required, conflicts, behind base, etc.)
// using MergeStateStatus and ReviewDecision. Otherwise returns "-".
func DetailsLabel(pr PR) string {
	if pr.Queue != nil {
		return queueDetail(pr)
	}
	if strings.ToUpper(pr.State) != "OPEN" || pr.IsDraft {
		return "-"
	}
	switch strings.ToUpper(pr.MergeStateStatus) {
	case "DIRTY":
		return "conflicts"
	case "BEHIND":
		return "behind base"
	case "UNSTABLE":
		// UNSTABLE = mergeable but commit status non-passing (failing OR
		// pending). The secondary statusCheckRollup query disambiguates.
		if label, ok := rollupCheckLabel(pr.CheckRollupState); ok {
			return label
		}
		return "checks failing"
	case "HAS_HOOKS":
		return "hooks pending"
	case "BLOCKED":
		return blockedDetail(pr)
	}
	return "-"
}

// rollupCheckLabel maps a statusCheckRollup state to the "checks ..." detail
// string. Returns (label, true) for FAILURE/ERROR/PENDING/EXPECTED. The
// SUCCESS case is handled by the caller because its meaning depends on
// MergeStateStatus (BLOCKED → branch protection, UNSTABLE → unreachable).
func rollupCheckLabel(state string) (string, bool) {
	switch strings.ToUpper(state) {
	case "FAILURE", "ERROR":
		return "checks failing", true
	case "PENDING", "EXPECTED":
		return "checks pending", true
	}
	return "", false
}

// blockedDetail explains why a BLOCKED PR is stuck: pending review, requested
// changes, or — when approved/no-decision — failing/pending checks vs pure
// branch-protection holds.
func blockedDetail(pr PR) string {
	switch strings.ToUpper(pr.ReviewDecision) {
	case "REVIEW_REQUIRED":
		return "review required"
	case "CHANGES_REQUESTED":
		return "changes requested"
	}
	if label, ok := rollupCheckLabel(pr.CheckRollupState); ok {
		return label
	}
	if strings.EqualFold(pr.CheckRollupState, "SUCCESS") {
		return "branch protection"
	}
	return "blocked"
}

// queueDetail formats the queue summary as "<state> #<pos> ~<eta>", omitting
// position/ETA when GitHub doesn't report them.
func queueDetail(pr PR) string {
	parts := []string{QueueLabelShort(pr)}
	if p := QueuePositionLabel(pr); p != "-" {
		parts = append(parts, "#"+p)
	}
	if e := ETALabel(pr); e != "-" {
		parts = append(parts, e)
	}
	return strings.Join(parts, " ")
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
