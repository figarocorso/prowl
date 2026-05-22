package data

// Stats summarises tracked PR counts and the active list's state breakdown.
// Reviewed counts are URL-only (no fetch) since those PRs are already archived.
type Stats struct {
	Active   int `json:"active"`
	Reviewed int `json:"reviewed"`
	Total    int `json:"total"`
	Open     int `json:"open"`
	Draft    int `json:"draft"`
	Blocked  int `json:"blocked"`
	Merged   int `json:"merged"`
	Closed   int `json:"closed"`
	Queued   int `json:"queued"`
	Errors   int `json:"errors,omitempty"`
}

// ComputeStats derives counts from the fetched active results and the raw
// reviewed-URL count.
func ComputeStats(active []Result, reviewedCount int) Stats {
	s := Stats{
		Active:   len(active),
		Reviewed: reviewedCount,
		Total:    len(active) + reviewedCount,
	}
	for _, r := range active {
		if r.Err != nil {
			s.Errors++
			continue
		}
		switch StatusLabel(r.PR) {
		case "open":
			s.Open++
		case "draft":
			s.Draft++
		case "open/blocked":
			s.Blocked++
		case "merged":
			s.Merged++
		case "closed":
			s.Closed++
		}
		if r.PR.Queue != nil {
			s.Queued++
		}
	}
	return s
}
