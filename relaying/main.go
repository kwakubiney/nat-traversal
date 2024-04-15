package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	var clientConfig ClientConfig
	var serverConfig ServerConfig
	var isServer bool

	flag.StringVar(&clientConfig.DestinationAddress, "s", "64.226.68.23:1234", "server address")
	flag.StringVar(&serverConfig.LocalAddress, "l", "1234", "local address")
	flag.StringVar(&clientConfig.NetworkName, "n", "A", "network you want to connect to")
	flag.BoolVar(&isServer, "srv", false, "server mode")

	flag.Parse()

	interruptChan := make(chan os.Signal, 1)
	signal.Notify(interruptChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-interruptChan
		fmt.Println("\nShutting down gracefully...")
		os.Exit(0)
	}()

	if isServer {
		var server = NewServer(serverConfig, 5*time.Second)
		err := server.Start()
		if err != nil {
			log.Printf("error starting server: %v", err)
		}
	} else {
		var client = NewClient(clientConfig, 5*time.Second)
		err := client.StartClient()
		if err != nil {
			log.Printf("error starting client: %v", err)
		}
	}

}
