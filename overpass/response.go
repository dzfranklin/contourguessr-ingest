package overpass

type Response struct {
	Generator string
	Count     int
	Nodes     map[int64]*Node
	Ways      map[int64]*Way
	// Relations aren't implemented because I don't need them yet
}

type Meta struct {
	ID   int64
	Tags map[string]string
}

type Node struct {
	Meta
	Lon float64
	Lat float64
}

type Way struct {
	Meta
	Nodes []*Node
}
