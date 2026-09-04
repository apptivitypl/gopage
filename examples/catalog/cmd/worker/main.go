package main

import (
	"github.com/syumai/workers"

	"github.com/apptivitypl/gopage"

	"github.com/apptivitypl/gopage/examples/catalog/internal/gen"
)

func main() {
	options := gen.Options()
	options.CacheBytes = 8 << 20
	app, err := gopage.New(options)
	if err != nil {
		panic(err)
	}
	workers.Serve(app.Handler())
}
