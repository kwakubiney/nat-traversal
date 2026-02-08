package main

import (
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
)

const (
	DefaultServerAddress = "64.226.68.23:1234"
	DefaultServerPort    = 1234
	HeartbeatInterval    = 10 * time.Second
	ServerTimeout        = 30 * time.Second
)

func main() {
	interruptChan := make(chan os.Signal, 1)
	signal.Notify(interruptChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-interruptChan
		fmt.Println("\nShutting down gracefully...")
		os.Exit(0)
	}()

	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "server":
		runServer(os.Args[2:])
	case "join":
		runClient(os.Args[2:])
	case "help", "--help", "-h":
		printUsage()
	default:
		fmt.Printf("Unknown command: %s\n\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`NAT Traversal - P2P Networking Through NATs

USAGE:
  nat-traversal <command> [options]

COMMANDS:
  server    Run the coordination server
  join      Join a network as a client

SERVER USAGE:
  nat-traversal server [port]

  Examples:
    nat-traversal server           # Listen on port 1234
    nat-traversal server 5000      # Listen on port 5000

CLIENT USAGE:
  nat-traversal join <network-name> [options]

  Options:
    --server, -s    Server address (default: ` + DefaultServerAddress + `)
    --name, -n      Client name for logging (default: random)
    --interactive, -i   Enable interactive chat mode

  Examples:
    nat-traversal join mynetwork
    nat-traversal join mynetwork --server 192.168.1.100:1234
    nat-traversal join mynetwork --name "Client-A"
    nat-traversal join mynetwork --name "Client-A" --interactive`)
}

func runServer(args []string) {
	port := DefaultServerPort

	if len(args) > 0 {
		var err error
		port, err = strconv.Atoi(args[0])
		if err != nil {
			fmt.Printf("Invalid port: %s\n", args[0])
			os.Exit(1)
		}
	}

	serverConn, err := net.ListenUDP("udp", &net.UDPAddr{Port: port})
	if err != nil {
		log.Fatalf("Error starting server: %v", err)
	}

	fmt.Printf("Coordination server listening on :%d\n", port)
	fmt.Println("Waiting for clients to connect...")

	server := NewServer(ServerConfig{LocalAddress: strconv.Itoa(port)}, ServerTimeout, serverConn)
	server.Start()
}

func runClient(args []string) {
	if len(args) < 1 {
		fmt.Println("Error: network name required")
		fmt.Println("\nUsage: nat-traversal join <network-name> [options]")
		os.Exit(1)
	}

	networkName := args[0]
	serverAddr := DefaultServerAddress
	clientName := ""
	interactive := false

	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--server", "-s":
			if i+1 >= len(args) {
				fmt.Println("Error: --server requires an address")
				os.Exit(1)
			}
			serverAddr = args[i+1]
			i++
		case "--name", "-n":
			if i+1 >= len(args) {
				fmt.Println("Error: --name requires a value")
				os.Exit(1)
			}
			clientName = args[i+1]
			i++
		case "--interactive", "-i":
			interactive = true
		}
	}

	// Generate a default name if not provided
	if clientName == "" {
		clientName = fmt.Sprintf("Peer-%d", os.Getpid()%1000)
	}

	server, err := net.ResolveUDPAddr("udp", serverAddr)
	if err != nil {
		log.Fatalf("Error resolving server address: %v", err)
	}

	// Port 0 = OS assigns a random available port.
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		log.Fatalf("Error creating socket: %v", err)
	}

	fmt.Printf("Joining network '%s' as \"%s\"\n", networkName, clientName)
	fmt.Printf("Coordination server: %s\n", serverAddr)

	config := ClientConfig{
		NetworkName:        networkName,
		DestinationAddress: serverAddr,
		Name:               clientName,
		Interactive:        interactive,
	}

	client := NewClient(config, HeartbeatInterval, conn, server)

	fmt.Println()
	if interactive {
		fmt.Println("Interactive mode enabled. Type messages and press Enter to send.")
	} else {
		fmt.Println("Waiting for peers...")
	}
	fmt.Println("Press Ctrl+C to exit")

	if err := client.StartClient(); err != nil {
		log.Fatalf("Client error: %v", err)
	}
}
