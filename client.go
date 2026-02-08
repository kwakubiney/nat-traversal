package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/netip"
	"os"
	"sync"
	"time"
)

// Client handles all client-side networking.
//
// Uses a single UDP socket for everything (heartbeats, probes, data, relay)
// so the NAT mapping stays consistent for hole punching.
type Client struct {
	Socket     *net.UDPConn
	ServerAddr *net.UDPAddr
	Duration   time.Duration
	Peers      map[netip.AddrPort]*PeerState
	Lock       sync.RWMutex
	Config     ClientConfig
}

func NewClient(config ClientConfig, duration time.Duration, conn *net.UDPConn, serverAddr *net.UDPAddr) *Client {
	return &Client{
		Socket:     conn,
		ServerAddr: serverAddr,
		Config:     config,
		Duration:   duration,
		Peers:      make(map[netip.AddrPort]*PeerState),
	}
}

func (c *Client) StartClient() error {
	ticker := time.NewTicker(c.Duration)
	defer c.Stop()
	defer ticker.Stop()

	heartbeatPayload := Heartbeat{NetworkName: c.Config.NetworkName, Name: c.Config.Name}

	payloadBytes, err := json.Marshal(heartbeatPayload)
	if err != nil {
		return fmt.Errorf("could not marshal heartbeat payload: %w", err)
	}

	heartbeatMessage, err := json.Marshal(Message{
		Type:    TypeHeartbeat,
		Payload: payloadBytes,
	})
	if err != nil {
		return fmt.Errorf("could not marshal heartbeat envelope: %w", err)
	}

	_, err = c.Socket.WriteToUDP(heartbeatMessage, c.ServerAddr)
	if err != nil {
		return fmt.Errorf("could not send initial heartbeat: %w", err)
	}
	log.Printf("[SEND] Heartbeat to server")

	errChHeartbeat := make(chan error)
	errChSocket := make(chan error)

	// Periodic heartbeats keep NAT mapping alive.
	go func() {
		for range ticker.C {
			_, err := c.Socket.WriteToUDP(heartbeatMessage, c.ServerAddr)
			if err != nil {
				errChHeartbeat <- err
			}
		}
	}()

	go func() {
		c.handleIncomingPackets(errChSocket)
	}()

	// Start chat loop based on mode
	if c.Config.Interactive {
		go c.startInteractiveChat()
	} else {
		go c.startChatLoop()
	}

	for {
		select {
		case e := <-errChHeartbeat:
			log.Printf("[Client] Heartbeat error: %v", e)
		case e := <-errChSocket:
			log.Printf("[Client] Socket error: %v", e)
		}
	}
}

func (c *Client) handleIncomingPackets(errCh chan<- error) {
	buf := make([]byte, 65535)

	for {
		n, addr, err := c.Socket.ReadFromUDP(buf)
		if err != nil {
			errCh <- err
			continue
		}

		// Ignore empty keepalive packets
		if n == 0 {
			continue
		}

		if addr.IP.Equal(c.ServerAddr.IP) && addr.Port == c.ServerAddr.Port {
			c.handleServerMessage(buf[:n])
			continue
		}

		var envelope Message
		if err := json.Unmarshal(buf[:n], &envelope); err != nil {
			// Silently ignore malformed packets (could be keepalives)
			continue
		}

		c.handlePeerMessage(envelope, addr)
	}
}

func (c *Client) handleServerMessage(data []byte) {
	var envelope Message
	if err := json.Unmarshal(data, &envelope); err != nil {
		log.Printf("[Client] Invalid server message: %v", err)
		return
	}

	switch envelope.Type {
	case TypePeerList:
		c.handlePeerList(envelope.Payload)
	case TypeRelay:
		c.handleRelayMessage(envelope.Payload)
	default:
		log.Printf("[Client] Unknown message type from server: %s", envelope.Type)
	}
}

func (c *Client) handlePeerList(payload json.RawMessage) {
	var peerList PeerList
	if err := json.Unmarshal(payload, &peerList); err != nil {
		log.Printf("[Client] Could not parse peer list: %v", err)
		return
	}

	if len(peerList.Peers) == 0 {
		log.Printf("[RECV] Peer list from server (no peers yet)")
		return
	}

	// Build a nice list of peer names
	names := make([]string, 0, len(peerList.Peers))
	for _, p := range peerList.Peers {
		if p.Name != "" {
			names = append(names, fmt.Sprintf("\"%s\"", p.Name))
		} else {
			names = append(names, p.PublicAddr.String())
		}
	}
	log.Printf("[RECV] Peer list from server: %v", names)

	for _, peer := range peerList.Peers {
		c.onPeerDiscovered(peer)
	}
}

// onPeerDiscovered initiates hole punching by sending a probe.
// If the probe isn't acked within 2s, falls back to relay.
func (c *Client) onPeerDiscovered(peer PeerInfo) {
	c.Lock.RLock()
	_, exists := c.Peers[peer.PublicAddr]
	c.Lock.RUnlock()
	if exists {
		return
	}

	c.Lock.Lock()
	if _, exists := c.Peers[peer.PublicAddr]; exists {
		c.Lock.Unlock()
		return
	}

	state := &PeerState{
		PublicAddr: peer.PublicAddr,
		Name:       peer.Name,
		IsRelay:    false,
		LastSeen:   0,
		ProbeAcked: false,
	}
	c.Peers[peer.PublicAddr] = state
	c.Lock.Unlock()

	peerName := peer.Name
	if peerName == "" {
		peerName = peer.PublicAddr.String()
	}
	log.Printf("[SEND] Probe to \"%s\"", peerName)

	udpAddr := net.UDPAddrFromAddrPort(peer.PublicAddr)
	c.sendMessage(TypeProbe, nil, udpAddr)

	time.AfterFunc(2*time.Second, func() {
		if !state.ProbeAcked {
			log.Printf("[INFO] Probe to \"%s\" timed out - using RELAY mode", peerName)
			state.IsRelay = true
		}
	})
}

func (c *Client) handlePeerMessage(envelope Message, addr *net.UDPAddr) {
	// Helper to get peer name
	getPeerName := func(ap netip.AddrPort) string {
		c.Lock.RLock()
		defer c.Lock.RUnlock()
		if state, ok := c.Peers[ap]; ok && state.Name != "" {
			return state.Name
		}
		return ap.String()
	}

	switch envelope.Type {
	case TypeProbe:
		c.sendMessage(TypeProbeAck, nil, addr)

		peerAP := addr.AddrPort()
		peerAP = netip.AddrPortFrom(peerAP.Addr().Unmap(), peerAP.Port())
		peerName := getPeerName(peerAP)

		log.Printf("[RECV] Probe from \"%s\"", peerName)
		log.Printf("[SEND] ProbeAck to \"%s\"", peerName)

		c.Lock.Lock()
		state, exists := c.Peers[peerAP]
		if !exists {
			state = &PeerState{
				PublicAddr: peerAP,
				Name:       "", // Will be updated if we get their info later
				IsRelay:    false,
				LastSeen:   time.Now().Unix(),
				ProbeAcked: true,
			}
			c.Peers[peerAP] = state
			log.Printf("[INFO] Direct connection established with \"%s\"!", peerName)
		} else {
			state.ProbeAcked = true
			state.IsRelay = false
			state.LastSeen = time.Now().Unix()
			log.Printf("[INFO] Direct connection established with \"%s\"!", peerName)
		}
		c.Lock.Unlock()

	case TypeProbeAck:
		peerAP := addr.AddrPort()
		peerAP = netip.AddrPortFrom(peerAP.Addr().Unmap(), peerAP.Port())
		peerName := getPeerName(peerAP)

		log.Printf("[RECV] ProbeAck from \"%s\"", peerName)

		c.Lock.Lock()
		if state, ok := c.Peers[peerAP]; ok {
			state.ProbeAcked = true
			state.LastSeen = time.Now().Unix()
			log.Printf("[INFO] Direct connection established with \"%s\"!", peerName)
		}
		c.Lock.Unlock()

	case TypeRelay:
		log.Printf("[RECV] Relayed message via server")
		c.handleRelayMessage(envelope.Payload)

	case TypeChat:
		var msg string
		if err := json.Unmarshal(envelope.Payload, &msg); err == nil {
			peerAP := addr.AddrPort()
			peerAP = netip.AddrPortFrom(peerAP.Addr().Unmap(), peerAP.Port())
			peerName := getPeerName(peerAP)
			log.Printf("[RECV] 💬 from \"%s\": %s", peerName, msg)
		}
	}
}

func (c *Client) startChatLoop() {
	ticker := time.NewTicker(8 * time.Second) // Slower for demo readability
	defer ticker.Stop()

	for range ticker.C {
		c.Lock.RLock()
		// Collect peers to chat with
		var peersToChat []*PeerState
		for _, peer := range c.Peers {
			if peer.ProbeAcked && !peer.IsRelay {
				peersToChat = append(peersToChat, peer)
			}
		}
		c.Lock.RUnlock()

		for _, peer := range peersToChat {
			msg := fmt.Sprintf("Hello from %s!", c.Config.Name)
			payload, _ := json.Marshal(msg)
			c.sendMessage(TypeChat, payload, net.UDPAddrFromAddrPort(peer.PublicAddr))

			peerName := peer.Name
			if peerName == "" {
				peerName = peer.PublicAddr.String()
			}
			log.Printf("[SEND] 💬 to \"%s\": %s", peerName, msg)
		}
	}
}

func (c *Client) startInteractiveChat() {
	// Background keepalive to prevent NAT mapping from expiring
	go func() {
		ticker := time.NewTicker(25 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			c.Lock.RLock()
			for _, peer := range c.Peers {
				if peer.ProbeAcked && !peer.IsRelay {
					// Send a silent keepalive (empty probe)
					c.Socket.WriteToUDP([]byte{}, net.UDPAddrFromAddrPort(peer.PublicAddr))
				}
			}
			c.Lock.RUnlock()
		}
	}()

	scanner := bufio.NewScanner(os.Stdin)

	for {
		fmt.Print("> ")
		if !scanner.Scan() {
			break
		}

		msg := scanner.Text()
		if msg == "" {
			continue
		}

		c.Lock.RLock()
		var peersToChat []*PeerState
		for _, peer := range c.Peers {
			if peer.ProbeAcked && !peer.IsRelay {
				peersToChat = append(peersToChat, peer)
			}
		}
		c.Lock.RUnlock()

		if len(peersToChat) == 0 {
			fmt.Println("No connected peers yet. Waiting for hole punch...")
			continue
		}

		payload, _ := json.Marshal(msg)
		for _, peer := range peersToChat {
			c.sendMessage(TypeChat, payload, net.UDPAddrFromAddrPort(peer.PublicAddr))

			peerName := peer.Name
			if peerName == "" {
				peerName = peer.PublicAddr.String()
			}
			log.Printf("[SEND] 💬 to \"%s\": %s", peerName, msg)
		}
	}
}

func (c *Client) handleRelayMessage(payload json.RawMessage) {
	var relayPayload RelayPayload
	if err := json.Unmarshal(payload, &relayPayload); err != nil {
		log.Printf("[Client] Invalid relay payload: %v", err)
		return
	}

	log.Printf("[Client] Received relayed data from %s (%d bytes)",
		relayPayload.Origin, len(relayPayload.Data))
}

// sendMessage sends directly to the peer, or via relay if hole punching failed.
func (c *Client) sendMessage(msgType MessageType, payload []byte, addr *net.UDPAddr) {
	targetAP := addr.AddrPort()

	c.Lock.RLock()
	state, ok := c.Peers[targetAP]
	shouldRelay := ok && state.IsRelay
	c.Lock.RUnlock()

	if shouldRelay {
		c.sendViaRelay(payload, targetAP)
		return
	}

	msg, _ := json.Marshal(Message{
		Type:    msgType,
		Payload: payload,
	})
	c.Socket.WriteToUDP(msg, addr)
}

func (c *Client) sendViaRelay(data []byte, target netip.AddrPort) {
	log.Printf("[SEND] Relay request to server for %s", target)

	relayPayload, _ := json.Marshal(RelayPayload{
		Target: target,
		Data:   data,
	})

	msg, _ := json.Marshal(Message{
		Type:    TypeRelay,
		Payload: relayPayload,
	})

	c.Socket.WriteToUDP(msg, c.ServerAddr)
}

func (c *Client) Stop() {
	c.Socket.Close()
}
