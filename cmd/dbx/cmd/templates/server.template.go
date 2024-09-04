package main

import (
	"fmt"
	"net/http"
)

func handler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "Hello from {{.PUP_NAME}}")
}

func main() {
	http.HandleFunc("/", handler)
	fmt.Println("Starting server on :8080")
	http.ListenAndServe(":8080", nil)
}
