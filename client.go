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
	ConnToServer    net.Conn
	Duration        time.Duration
	NetworkMap      []netip.AddrPort
	PeerConnections []net.Conn
	Config          ClientConfig
}

type Heartbeat struct {
	NetworkName string
}

func NewClient(config ClientConfig, duration time.Duration, conn net.Conn) Client {
	return Client{
		ConnToServer:    conn,
		Config:          config,
		PeerConnections: []net.Conn{},
		Duration:        duration,
		NetworkMap:      make([]netip.AddrPort, 0),
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

	_, err = c.ConnToServer.Write(heartbeatMessage)
	if err != nil {
		return fmt.Errorf("could not send initial request to server: %w", err)
	}

	errChForHeartbeat := make(chan error)
	errChForServerRequest := make(chan error)
	go func() {
		for range tickerForHeartbeat.C {
			_, heartbeatErr := c.ConnToServer.Write(heartbeatMessage)
			if heartbeatErr != nil {
				errChForHeartbeat <- heartbeatErr
			}
		}
	}()
	go func() {
		for {
			buf := make([]byte, 65535)
			n, err := c.ConnToServer.Read(buf)
			if err != nil {
				errChForServerRequest <- err
			}
			currentNetworkMap := make([]netip.AddrPort, 0)
			err = json.Unmarshal(buf[:n], &currentNetworkMap)
			if err != nil {
				errChForServerRequest <- err
			}

			for _, addr := range currentNetworkMap {
				if !slices.Contains(c.NetworkMap, addr) {
					c.NetworkMap = append(c.NetworkMap, addr)
					conn, err := net.Dial("udp", addr.String())
					if err != nil {
						log.Println("error dialing to peer:", err)
						continue
					}
					//send your tun address to the peer after dialing
					//you know the remote address so send tun ip via udp
					//you should also listen on the same connection for a reply
					//associate the tun with netIP.AddrPort received
					//maybe data structure should be map[tunIP]*conn
					//is udp reliable for this? is there a more reliable protocol?

					//who controls who? who sends the tun ip first?
					//what if the peer is behind a NAT?
					c.PeerConnections = append(c.PeerConnections, conn)
				}
			}
		}
	}()

	for {
		select {
		case e := <-errChForHeartbeat:
			log.Println("error from heartbeat:", e)

		case e := <-errChForServerRequest:
			log.Println("error from initial request:", e)
		}
	}
}

func (c *Client) Stop() {
	c.ConnToServer.Close()
	for _, conn := range c.PeerConnections {
		conn.Close()
	}
}
