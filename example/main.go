package main

import (
	"log"

	client "github.com/omarcosr/teleport/client"
	server "github.com/omarcosr/teleport/server"
)

func main() {
	go func() {
		if err := runServer(); err != nil {
			log.Fatal(err)
		}
	}()

	if err := runClient(); err != nil {
		log.Fatal(err)
	}
}

func runServer() error {
	server.Start("hash-key", "127.0.0.1:1111")
	return nil
}
func runClient() error {
	return client.ListenAndServe(client.Client{
		Key:           "hash-key",
		BindPort:      "8888",
		LocalAddress:  "127.0.0.1:22",
		ServerAddress: "127.0.0.1:1111",
	})
}
