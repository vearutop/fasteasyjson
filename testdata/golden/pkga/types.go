package pkga

// Status is a named string type.
type Status string

const (
	StatusActive Status = "active"
	StatusClosed Status = "closed"
)

//easyjson:json
type Widget struct {
	Name   string   `json:"name"`
	Count  int      `json:"count,omitempty"`
	Tags   []string `json:"tags,omitempty"`
	Status Status   `json:"status"`
}
