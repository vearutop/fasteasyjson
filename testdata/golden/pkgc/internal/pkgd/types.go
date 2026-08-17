package pkgd

// Level is a named int type.
type Level int

//easyjson:json
type Alpha struct {
	Value string `json:"value"`
	Level Level  `json:"level,omitempty"`
}
