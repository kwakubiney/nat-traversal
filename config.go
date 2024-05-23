package main

type ClientConfig struct {
	LocalAddress       string
	DestinationAddress string
	Global             bool
	ServerMode         bool
	NetworkName        string
	ClientTunIP        string
}

type ServerConfig struct {
	LocalAddress string
	LocalPort    string
}
