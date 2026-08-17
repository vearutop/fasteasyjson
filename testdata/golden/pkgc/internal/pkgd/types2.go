package pkgd

//easyjson:json
type Beta struct {
	Count int      `json:"count"`
	Names []string `json:"names,omitempty"`
	Ref   *Alpha   `json:"ref,omitempty"`
}
