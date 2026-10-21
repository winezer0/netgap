package guiapp

type Alert struct {
	Type       string `json:"type"`
	Message    string `json:"message"`
	Timestamp  int64  `json:"timestamp"`
	ObservedAt int64  `json:"observedAt"`
	EventID    uint64 `json:"eventId"`
	RunID      uint64 `json:"runId"`
	IP         string `json:"ip"`
	Location   string `json:"location"`
	Path       string `json:"path"`
	Protection string `json:"protection"`
	Active     bool   `json:"active"`
	Historical bool   `json:"historical"`
}
