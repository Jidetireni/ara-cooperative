package main

import "fmt"

func main() {
	server, cleanup := NewServer()
	defer cleanup()
	server.Start()
	fmt.Println("Server started successfully")
}
