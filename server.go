package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/netip"
	"sync"
	"time"
)

// Server is the coordination server.
// It tracks peers, broadcasts peer lists, and relays data when direct P2P fails.
type Server struct {
	Config ServerConfig

	// network name -> (public addr -> peer info)
	NetworkTopology  map[string]map[netip.AddrPort]*ServerPeerInfo
	Lock             sync.Mutex
	Duration         time.Duration
	ServerListenConn *net.UDPConn
}

type ServerPeerInfo struct {
	PublicAddr netip.AddrPort
	Name       string
	LastSeen   time.Time
}

func NewServer(config ServerConfig, duration time.Duration, conn *net.UDPConn) *Server {
	return &Server{
		Config:           config,
		NetworkTopology:  make(map[string]map[netip.AddrPort]*ServerPeerInfo),
		Duration:         duration,
		ServerListenConn: conn,
	}
}

func (s *Server) Start() {
	log.Printf("[Server] Coordination server started on %s", s.ServerListenConn.LocalAddr())
	defer s.Stop()

	go s.cleanupStalePeers()

	buf := make([]byte, 65535)

	for {
		n, addr, err := s.ServerListenConn.ReadFromUDP(buf)
		if err != nil {
			log.Printf("[Server] Read error: %v", err)
			continue
		}

		var envelope Message
		if err := json.Unmarshal(buf[:n], &envelope); err != nil {
			log.Printf("[Server] Invalid message from %s: %v", addr, err)
			continue
		}

		switch envelope.Type {
		case TypeHeartbeat:
			s.handleHeartbeat(envelope.Payload, addr)
		case TypeRelay:
			log.Printf("[RECV] Relay request")
			s.handleRelay(envelope.Payload, addr)
		default:
			log.Printf("[Server] Unknown message type '%s' from %s", envelope.Type, addr)
		}
	}
}

func (s *Server) handleHeartbeat(payload json.RawMessage, addr *net.UDPAddr) {
	var hb Heartbeat
	if err := json.Unmarshal(payload, &hb); err != nil {
		log.Printf("[Server] Invalid heartbeat payload: %v", err)
		return
	}

	isNew := s.addOrUpdatePeer(hb.NetworkName, hb.Name, addr)
	if isNew {
		s.broadcastPeerList(hb.NetworkName)
	}
}

// addOrUpdatePeer returns true if this is a new peer.
func (s *Server) addOrUpdatePeer(networkName string, peerName string, addr *net.UDPAddr) bool {
	s.Lock.Lock()
	defer s.Lock.Unlock()

	if s.NetworkTopology[networkName] == nil {
		s.NetworkTopology[networkName] = make(map[netip.AddrPort]*ServerPeerInfo)
	}

	network := s.NetworkTopology[networkName]

	// Unmap to ensure consistent IPv4/IPv6 handling
	addrPort := addr.AddrPort()
	addrPort = netip.AddrPortFrom(addrPort.Addr().Unmap(), addrPort.Port())

	if peer, exists := network[addrPort]; exists {
		peer.LastSeen = time.Now()
		return false
	}

	log.Printf("[INFO] New peer joined: \"%s\"", peerName)

	network[addrPort] = &ServerPeerInfo{
		PublicAddr: addrPort,
		Name:       peerName,
		LastSeen:   time.Now(),
	}

	return true
}

// broadcastPeerList sends the full peer list to every member in the network.
func (s *Server) broadcastPeerList(networkName string) {
	s.Lock.Lock()
	defer s.Lock.Unlock()

	network := s.NetworkTopology[networkName]
	if network == nil {
		return
	}

	// Send each peer a list that excludes themselves.
	for addrPort, recipient := range network {
		peers := make([]PeerInfo, 0, len(network)-1)
		for _, peer := range network {
			if peer.PublicAddr == addrPort {
				continue
			}
			peers = append(peers, PeerInfo{PublicAddr: peer.PublicAddr, Name: peer.Name})
		}

		payload, err := json.Marshal(PeerList{Peers: peers})
		if err != nil {
			log.Printf("[Server] Error marshaling peer list: %v", err)
			continue
		}

		msg, err := json.Marshal(Message{
			Type:    TypePeerList,
			Payload: payload,
		})
		if err != nil {
			log.Printf("[Server] Error marshaling message: %v", err)
			continue
		}

		s.ServerListenConn.WriteToUDPAddrPort(msg, addrPort)

		// Build nice log of peer names
		names := make([]string, 0, len(peers))
		for _, p := range peers {
			names = append(names, fmt.Sprintf("\"%s\"", p.Name))
		}
		log.Printf("[SEND] Peer list to \"%s\": %v", recipient.Name, names)
	}
}

// handleRelay forwards data to the target peer.
func (s *Server) handleRelay(payload json.RawMessage, from *net.UDPAddr) {
	var relayPayload RelayPayload
	if err := json.Unmarshal(payload, &relayPayload); err != nil {
		log.Printf("[Server] Invalid relay payload: %v", err)
		return
	}

	// Unmap origin address
	fromAddr := from.AddrPort()
	fromAddr = netip.AddrPortFrom(fromAddr.Addr().Unmap(), fromAddr.Port())
	relayPayload.Origin = fromAddr

	newPayload, err := json.Marshal(relayPayload)
	if err != nil {
		log.Printf("[Server] Error marshaling relay payload: %v", err)
		return
	}

	msg, err := json.Marshal(Message{
		Type:    TypeRelay,
		Payload: newPayload,
	})
	if err != nil {
		log.Printf("[Server] Error marshaling relay message: %v", err)
		return
	}

	// Get names for nice logging
	s.Lock.Lock()
	fromName := fromAddr.String()
	toName := relayPayload.Target.String()
	for _, network := range s.NetworkTopology {
		if peer, ok := network[fromAddr]; ok && peer.Name != "" {
			fromName = peer.Name
		}
		if peer, ok := network[relayPayload.Target]; ok && peer.Name != "" {
			toName = peer.Name
		}
	}
	s.Lock.Unlock()

	log.Printf("[RELAY] \"%s\" -> \"%s\"", fromName, toName)
	s.ServerListenConn.WriteToUDPAddrPort(msg, relayPayload.Target)
}

// cleanupStalePeers removes peers that haven't heartbeated within 3x the timeout.
func (s *Server) cleanupStalePeers() {
	ticker := time.NewTicker(s.Duration)
	defer ticker.Stop()

	for range ticker.C {
		s.Lock.Lock()

		now := time.Now()
		timeout := s.Duration * 3

		for networkName, network := range s.NetworkTopology {
			changed := false

			for addrPort, peer := range network {
				if now.Sub(peer.LastSeen) > timeout {
					log.Printf("[INFO] Removing stale peer: \"%s\"", peer.Name)
					delete(network, addrPort)
					changed = true
				}
			}

			if changed {
				s.Lock.Unlock()
				s.broadcastPeerList(networkName)
				s.Lock.Lock()
			}

			if len(network) == 0 {
				delete(s.NetworkTopology, networkName)
			}
		}

		s.Lock.Unlock()
	}
}

func (s *Server) Stop() {
	s.ServerListenConn.Close()
}
