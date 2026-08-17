package pkgb

// Quantity is a named int type.
type Quantity int

//easyjson:json
type Gadget struct {
	ID       string     `json:"id"`
	Price    float64    `json:"price"`
	Extra    *string    `json:"extra,omitempty"`
	Qty      Quantity   `json:"qty,omitempty"`
	Specs    *Spec      `json:"specs,omitempty"`
	Specs2   Spec       `json:"specs2,omitzero"`
	Variants []*Variant `json:"variants,omitempty"`
}

type Spec struct {
	Weight float64 `json:"weight"`
	Color  string  `json:"color"`
}

type Variant struct {
	SKU   string `json:"sku"`
	Stock int    `json:"stock,omitempty"`
}
