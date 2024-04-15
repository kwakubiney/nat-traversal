package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"strconv"
	"sync"
	"time"
)

type Server struct {
	Config          ServerConfig
	NetworkTopology map[string][]*net.UDPAddr
	Lock            sync.Mutex
	Duration        time.Duration
}

func NewServer(config ServerConfig, duration time.Duration) Server {
	return Server{
		Config:          config,
		NetworkTopology: map[string][]*net.UDPAddr{},
		Duration:        duration,
	}
}

func (s *Server) Start() error {
	localAddress, _ := strconv.Atoi(s.Config.LocalAddress)
	serverConn, err := net.ListenUDP("udp", &net.UDPAddr{Port: localAddress})

	if err != nil {
		return fmt.Errorf("failed to listen on port : %w", err)
	}
	for {
		buf := make([]byte, 1500)
		fmt.Println("listening...")
		n, addr, err := serverConn.ReadFromUDP(buf)
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
		s.addAddress(message.NetworkName, addr)
		fmt.Println("received message from", addr)
	}
}

func (s *Server) addAddress(networkName string, addr *net.UDPAddr) {
	s.Lock.Lock()
	defer s.Lock.Unlock()

	for _, existingAddr := range s.NetworkTopology[networkName] {
		if existingAddr.IP.Equal(addr.IP) && existingAddr.Port == addr.Port {
			return
		}
	}
	s.NetworkTopology[networkName] = append(s.NetworkTopology[networkName], addr)
}
