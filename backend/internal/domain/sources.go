package domain

// Source is the stable API representation shared by knowledge documents and
// growth notes. It deliberately contains only citation-safe preview data.
type Source struct {
	Type          string  `json:"source_type"`
	ID            int64   `json:"source_id"`
	Title         string  `json:"title"`
	Snippet       *string `json:"snippet,omitempty"`
	Heading       *string `json:"heading,omitempty"`
	PageFrom      *int    `json:"page_from,omitempty"`
	PageTo        *int    `json:"page_to,omitempty"`
	Rank          int     `json:"rank"`
	SourceDeleted bool    `json:"source_deleted"`
}
