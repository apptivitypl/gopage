package main

import (
	"log"
	"os"

	"github.com/apptivitypl/rill"

	"github.com/apptivitypl/rill/examples/blog/internal/gen"
)

func main() {
	options := gen.Options()
	options.CacheBytes = 32 << 20
	app, err := rill.New(options)
	if err != nil {
		log.Fatal(err)
	}
	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = ":8080"
	}
	log.Fatal(rill.Serve(addr, app))
}
