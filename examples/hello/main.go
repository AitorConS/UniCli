package main

import (
	"fmt"
	"os"
	"time"
)

func main() {
	fmt.Println("Hello from unikernel!")
	fmt.Printf("time : %s\n", time.Now().UTC().Format(time.RFC3339))
	fmt.Printf("pid  : %d\n", os.Getpid())
	os.Exit(0)
}
