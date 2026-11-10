package evidence

// Source is a web result returned by the configured search provider. The ID is
// assigned by this application and is the only identifier the model may cite.
type Source struct {
	ID          string  `json:"id"`
	Title       string  `json:"title"`
	URL         string  `json:"url"`
	Domain      string  `json:"domain,omitempty"`
	Snippet     string  `json:"snippet"`
	PublishedAt string  `json:"published_at,omitempty"`
	Score       float64 `json:"score,omitempty"`
}
