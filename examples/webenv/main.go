package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
)

func main() {
	http.HandleFunc("/", handler)
	log.Println("listening on :4333")
	if err := http.ListenAndServe(":4333", nil); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
}

func handler(w http.ResponseWriter, r *http.Request) {
	message := os.Getenv("MESSAGE")
	if message == "" {
		message = "(not set)"
	}
	fmt.Fprintf(w, "MESSAGE=%s\n", message)
}
