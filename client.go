package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/netip"
	"slices"
	"time"
)

type Client struct {
	Socket     *net.UDPConn
	ServerAddr *net.UDPAddr
	Duration   time.Duration
	NetworkMap []netip.AddrPort
	Config     ClientConfig
}

type Heartbeat struct {
	NetworkName string
}

func NewClient(config ClientConfig, duration time.Duration, conn *net.UDPConn, serverAddr *net.UDPAddr) Client {
	return Client{
		Socket:     conn,
		ServerAddr: serverAddr,
		Config:     config,
		Duration:   duration,
		NetworkMap: make([]netip.AddrPort, 0),
	}
}

func (c *Client) StartClient() error {
	tickerForHeartbeat := time.NewTicker(c.Duration)
	defer c.Stop()
	defer tickerForHeartbeat.Stop()

	heartbeatMessage, err := json.Marshal(Heartbeat{NetworkName: c.Config.NetworkName})
	if err != nil {
		return fmt.Errorf("could not marshal heartbeat message: %w", err)
	}

	_, err = c.Socket.WriteToUDP(heartbeatMessage, c.ServerAddr)
	if err != nil {
		return fmt.Errorf("could not send initial request to server: %w", err)
	}

	errChForHeartbeat := make(chan error)
	errChForSocket := make(chan error)

	go func() {
		for range tickerForHeartbeat.C {
			_, heartbeatErr := c.Socket.WriteToUDP(heartbeatMessage, c.ServerAddr)
			if heartbeatErr != nil {
				errChForHeartbeat <- heartbeatErr
			}
		}
	}()

	go func() {
		for {
			buf := make([]byte, 65535)
			n, addr, err := c.Socket.ReadFromUDP(buf)
			if err != nil {
				errChForSocket <- err
				continue
			}

			// Check if message is from server
			if addr.IP.Equal(c.ServerAddr.IP) && addr.Port == c.ServerAddr.Port {
				currentNetworkMap := make([]netip.AddrPort, 0)
				err = json.Unmarshal(buf[:n], &currentNetworkMap)
				if err != nil {
					log.Printf("Received invalid JSON from server: %v", err)
					continue
				}

				for _, peerAddrPort := range currentNetworkMap {
					if !slices.Contains(c.NetworkMap, peerAddrPort) {
						c.NetworkMap = append(c.NetworkMap, peerAddrPort)

						// Convert netip.AddrPort to *net.UDPAddr
						peerAddr := net.UDPAddrFromAddrPort(peerAddrPort)

						// HOLE PUNCHING: Send a dummy packet immediately!
						// This tells our NAT: "Expect traffic from this IP:Port"
						log.Printf("New peer discovered: %s. Punching hole...", peerAddr)
						_, err := c.Socket.WriteToUDP([]byte("hello"), peerAddr)
						if err != nil {
							log.Printf("Error punching hole to %s: %v", peerAddr, err)
						}
					}
				}
				continue
			}

			// Message is from a Peer
			log.Printf("Received P2P message from %s: %s", addr, string(buf[:n]))
		}
	}()

	for {
		select {
		case e := <-errChForHeartbeat:
			log.Println("error from heartbeat:", e)
		case e := <-errChForSocket:
			log.Println("error from socket read:", e)
		}
	}
}

func (c *Client) Stop() {
	c.Socket.Close()
}
