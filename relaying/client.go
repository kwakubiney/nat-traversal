package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"time"
)

type Client struct {
	NetworkName     string
	Duration        time.Duration
	PeerConnections []net.Conn
	Config          ClientConfig
}

type Heartbeat struct {
	NetworkName string
}

func NewClient(config ClientConfig, duration time.Duration) Client {
	return Client{
		Config:          config,
		PeerConnections: []net.Conn{},
		Duration:        duration,
	}
}

func (c *Client) StartClient() error {
	conn, err := net.Dial("udp", c.Config.DestinationAddress)
	if err != nil {
		return fmt.Errorf("could not connect to remote server: %w", err)
	}
	tickerForHeartbeat := time.NewTicker(c.Duration)
	defer conn.Close()
	defer tickerForHeartbeat.Stop()

	heartbeatMessage, err := json.Marshal(Heartbeat{NetworkName: c.NetworkName})

	if err != nil {
		return fmt.Errorf("could not marshal heartbeat message: %w", err)
	}

	errChForHeartbeat := make(chan error)
	errChForInitialRequest := make(chan error)
	go func() {
		for {
			select {
			case <-tickerForHeartbeat.C:
				_, heartbeatErr := conn.Write((heartbeatMessage))
				if heartbeatErr != nil {
					errChForHeartbeat <- heartbeatErr
				}
			}
		}
	}()
	go func() {
		for {
			select {
			case <-tickerForHeartbeat.C:
				buf := make([]byte, 1500)
				_, err := conn.Write(buf)
				if err != nil {
					errChForInitialRequest <- err
				}
			}
		}
	}()

	for {
		select {
		case e := <-errChForHeartbeat:
			log.Println("error from heartbeat:", e)

		case e := <-errChForInitialRequest:
			log.Println("error from initial request:", e)
		}
	}
}
