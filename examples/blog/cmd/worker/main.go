package main

import (
	"github.com/syumai/workers"

	"github.com/apptivitypl/rill"

	"github.com/apptivitypl/rill/examples/blog/internal/gen"
)

func main() {
	options := gen.Options()
	options.CacheBytes = 8 << 20
	app, err := rill.New(options)
	if err != nil {
		panic(err)
	}
	workers.Serve(app.Handler())
}
