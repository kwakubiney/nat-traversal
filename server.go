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

type Server struct {
	Config           ServerConfig
	NetworkTopology  map[string][]netip.AddrPort
	Lock             sync.Mutex
	Duration         time.Duration
	NewMemberAlert   chan struct{}
	ServerListenConn *net.UDPConn
}

func NewServer(config ServerConfig, duration time.Duration, conn *net.UDPConn) Server {
	return Server{
		Config:           config,
		NetworkTopology:  map[string][]netip.AddrPort{},
		Duration:         duration,
		NewMemberAlert:   make(chan struct{}),
		ServerListenConn: conn,
	}
}

func (s *Server) Start() {
	fmt.Println("Listening for connections...")
	defer s.Stop()
	for {
		buf := make([]byte, 65535)
		n, addr, err := s.ServerListenConn.ReadFromUDP(buf)
		buf = buf[:n]
		if err != nil {
			log.Println(fmt.Errorf("error during reading udp buffer: %w", err))
			continue
		}
		var message Heartbeat
		err = json.Unmarshal(buf, &message)
		if err != nil {
			log.Println(fmt.Errorf("error during reading udp buffer: %w", err))
			continue
		}
		var isNewMember = s.addAddressAndDetermineExistence(message.NetworkName, addr)

		if isNewMember {
			resp, err := json.Marshal(s.NetworkTopology[message.NetworkName])
			if err != nil {
				log.Println(fmt.Errorf("error during marshalling udp buffer: %w", err))
				continue
			}
			for _, lookedUpAddress := range s.NetworkTopology[message.NetworkName] {
				if lookedUpAddress == addr.AddrPort() {
					continue
				}
				_, err = s.ServerListenConn.WriteToUDPAddrPort(resp, lookedUpAddress)
				if err != nil {
					log.Println(fmt.Errorf("error during writing udp buffer: %w", err))
					continue
				}
			}
		}
	}
}

func (s *Server) addAddressAndDetermineExistence(networkName string, addr *net.UDPAddr) bool {
	s.Lock.Lock()
	defer s.Lock.Unlock()

	for _, existingAddr := range s.NetworkTopology[networkName] {
		if existingAddr == addr.AddrPort() {
			return false
		}
	}

	log.Printf("New peer for network %s detected: %v:%d", networkName, addr.IP.To4(), addr.Port)
	s.NetworkTopology[networkName] = append(s.NetworkTopology[networkName], addr.AddrPort())
	return true
}

func (s *Server) Stop() {
	s.ServerListenConn.Close()
}
