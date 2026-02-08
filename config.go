package main

type ClientConfig struct {
	NetworkName        string
	DestinationAddress string
	Name               string
	Interactive        bool
}

type ServerConfig struct {
	LocalAddress string
}
