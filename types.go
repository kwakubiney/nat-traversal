package main

import (
	"encoding/json"
	"net/netip"
)

type MessageType string

const (
	TypeHeartbeat MessageType = "heartbeat"
	TypePeerList  MessageType = "peerList"
	TypeProbe     MessageType = "probe"
	TypeProbeAck  MessageType = "probe_ack"
	TypeRelay     MessageType = "relay"
	TypeChat      MessageType = "chat"
)

type Message struct {
	Type    MessageType     `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

type Heartbeat struct {
	NetworkName string `json:"network_name"`
	Name        string `json:"name"`
}

type PeerInfo struct {
	PublicAddr netip.AddrPort `json:"public_addr"`
	Name       string         `json:"name"`
}

type PeerList struct {
	Peers []PeerInfo `json:"peers"`
}

type RelayPayload struct {
	Target netip.AddrPort `json:"target"`
	Origin netip.AddrPort `json:"origin"`
	Data   []byte         `json:"data"`
}

type PeerState struct {
	PublicAddr netip.AddrPort
	Name       string
	IsRelay    bool
	LastSeen   int64
	ProbeAcked bool
}
