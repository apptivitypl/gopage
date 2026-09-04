package main

import (
	"log"
	"os"

	"github.com/apptivitypl/gopage"

	"github.com/apptivitypl/gopage/examples/catalog/internal/gen"
)

func main() {
	options := gen.Options()
	options.CacheBytes = 32 << 20
	app, err := gopage.New(options)
	if err != nil {
		log.Fatal(err)
	}
	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = ":8080"
	}
	log.Fatal(gopage.Serve(addr, app))
}
