package pkge

// Kind is a named string type.
type Kind string

//easyjson:json
type Gamma struct {
	*Common
	Flag bool              `json:"flag"`
	Meta map[string]string `json:"meta,omitempty"`
	Kind Kind              `json:"kind,omitempty"`
	Sub  *SubGamma         `json:"sub,omitempty"`
}

// Common is embedded as a pointer anonymously in Gamma, so its fields are
// promoted to the top level of the generated JSON.
type Common struct {
	Note string `json:"note"`
}

type SubGamma struct {
	Detail string `json:"detail,omitempty"`
}
