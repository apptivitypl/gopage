package itemcard

type Props struct {
	Item Item
}

type Item struct {
	ID      string
	Name    string
	City    string
	Price   int64
	Area    int64
	Summary string
}
