package main

import (
	"fmt"
	"net/http"
	"os"
)

func main() {
	address := os.Getenv("ADDR")
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "path=%s host=%s tag=%s", r.URL.Path, r.Host, os.Getenv("TAG"))
	})
	_ = http.ListenAndServe(address, nil)
}
