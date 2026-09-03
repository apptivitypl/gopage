package catalog

import (
	"sort"
	"strings"
)

type Item struct {
	ID      string
	Name    string
	City    string
	Price   int64
	Area    int64
	Status  string
	Summary string
}

var items = []Item{
	{ID: "1", Name: "attic on the square", City: "krakow", Price: 640000, Area: 48, Status: "active",
		Summary: "two rooms under the roof, one window facing the market"},
	{ID: "2", Name: "flat by the river", City: "warsaw", Price: 890000, Area: 62, Status: "active",
		Summary: "quiet street, a tram stop at the corner"},
	{ID: "3", Name: "house with a garden", City: "krakow", Price: 1450000, Area: 140, Status: "paused",
		Summary: "needs a new roof, the garden is already there"},
	{ID: "4", Name: "studio near the park", City: "gdansk", Price: 410000, Area: 29, Status: "archived",
		Summary: "small, bright, and a short walk from the water"},
}

func All(city string) []Item {
	var found []Item
	for _, item := range items {
		if city != "" && item.City != city {
			continue
		}
		found = append(found, item)
	}
	sort.Slice(found, func(i, j int) bool { return found[i].Price < found[j].Price })
	return found
}

func Cities() []string {
	seen := map[string]bool{}
	var names []string
	for _, item := range items {
		if !seen[item.City] {
			seen[item.City] = true
			names = append(names, item.City)
		}
	}
	sort.Strings(names)
	return names
}

func Get(id string) (Item, bool) {
	for _, item := range items {
		if item.ID == id {
			return item, true
		}
	}
	return Item{}, false
}

func Known(city string) bool {
	return city == "" || strings.TrimSpace(city) != "" && contains(Cities(), city)
}

func contains(values []string, value string) bool {
	for _, held := range values {
		if held == value {
			return true
		}
	}
	return false
}
