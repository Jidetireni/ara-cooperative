package main

import (
	"fmt"
	"os"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

}

func run() error {
	server, cleanup, err := NewServer()
	if err != nil {
		return fmt.Errorf("failed to create server: %w", err)
	}
	defer cleanup()

	server.Start()
	return nil
}
