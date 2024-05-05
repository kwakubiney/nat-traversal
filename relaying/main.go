package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"strconv"
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
		localAddress, _ := strconv.Atoi(serverConfig.LocalAddress)
		serverConn, err := net.ListenUDP("udp", &net.UDPAddr{Port: localAddress})
		if err != nil {
			log.Printf("error starting server: %v", err)
		}
		var server = NewServer(serverConfig, 5*time.Second, serverConn)
		server.Start()

	} else {
		conn, err := net.Dial("udp", clientConfig.DestinationAddress)
		if err != nil {
			log.Printf("error dialing server: %v", err)
		}
		var client = NewClient(clientConfig, 200*time.Second, conn)
		err = client.StartClient()
		if err != nil {
			log.Printf("error starting client: %v", err)
		}
	}
}
