package main

type ClientConfig struct {
	LocalAddress       string
	DestinationAddress string
	Global             bool
	ServerMode         bool
	NetworkName        string
}

type ServerConfig struct {
	LocalAddress string
	LocalPort    string
}
