package main

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
)

func main() {
	if child := os.Getenv("CHILD"); child != "" {
		_ = http.ListenAndServe(child, nil)
		return
	}
	if spawn := os.Getenv("SPAWN"); spawn != "" {
		command := exec.Command(os.Args[0])
		command.Env = append(os.Environ(), "CHILD="+spawn)
		_ = command.Start()
	}
	address := os.Getenv("ADDR")
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "path=%s host=%s tag=%s", r.URL.Path, r.Host, os.Getenv("TAG"))
	})
	_ = http.ListenAndServe(address, nil)
}
