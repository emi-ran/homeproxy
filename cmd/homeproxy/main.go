package main

import (
	"log"
	"os"

	"homeproxy/internal/proxy"
)

func main() {
	if err := proxy.RunCLI(os.Args[1:]); err != nil {
		log.Fatal(err)
	}
}
