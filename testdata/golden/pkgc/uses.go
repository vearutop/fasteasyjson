package pkgc

//easyjson:json
type Container struct {
	Label string
	Items []Item
}

type Item struct {
	Key   string
	Value int
}
