package pkgc

//easyjson:json
type Container struct {
	Meta
	Label string          `json:"label"`
	Items []Item          `json:"items,omitempty"`
	Index map[string]Item `json:"index,omitempty"`
}

// Meta is embedded anonymously in Container, so its fields are promoted to
// the top level of the generated JSON.
type Meta struct {
	CreatedBy string `json:"created_by"`
}

type Item struct {
	Key   string `json:"key"`
	Value int    `json:"value,omitempty"`
}
