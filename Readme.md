# NAT Traversal

A peer-to-peer chat application demonstrating UDP hole punching to establish direct connections between peers behind NAT/firewalls.

## How It Works

1. **Coordination Server**: A lightweight server that peers connect to for discovery. It only facilitates initial peer exchange—no messages are relayed through it.

2. **UDP Hole Punching**: When two peers want to connect, they simultaneously send UDP packets to each other's public IP:port (learned from the coordination server). This "punches holes" through their NATs, allowing direct communication.

3. **Direct P2P**: Once the hole is punched, peers communicate directly. The coordination server can be killed and chat continues.

```
┌─────────────┐                              ┌─────────────┐
│   Peer A    │                              │   Peer B    │
│ (behind NAT)│                              │ (behind NAT)│
└──────┬──────┘                              └──────┬──────┘
       │                                            │
       │  1. Register with server                   │
       ├──────────────────┐    ┌───────────────────┤
       │                  ▼    ▼                   │
       │            ┌───────────────┐              │
       │            │  Coordination │              │
       │            │    Server     │              │
       │            └───────────────┘              │
       │                                           │
       │  2. Exchange peer info                    │
       │◄─────────────────────────────────────────►│
       │                                           │
       │  3. Simultaneous UDP punch                │
       │◄─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─►│
       │                                           │
       │  4. Direct P2P communication              │
       │◄══════════════════════════════════════════►│
       │      (server no longer needed)            │
```

## Usage

### Build

```bash
go build -o nat-traversal .
```

### Run Coordination Server

```bash
./nat-traversal server 8080
```

### Join a Network

```bash
# Basic usage
./nat-traversal join <network-name> --server <host:port>

# With peer name and interactive mode
./nat-traversal join my-network --server 1.2.3.4:8080 --name "Alice" --interactive
```

**Flags:**
- `--server` - Coordination server address (required)
- `--name` - Human-readable name for this peer (optional)
- `--interactive` - Enable interactive chat mode (optional)

### Example Session

**Terminal 1 (Server):**
```bash
./nat-traversal server 8080
```

**Terminal 2 (Peer A):**
```bash
./nat-traversal join demo --server your-server:8080 --name "Alice" --interactive
```

**Terminal 3 (Peer B):**
```bash
./nat-traversal join demo --server your-server:8080 --name "Bob" --interactive
```

Once connected, type messages in interactive mode. They flow directly between peers.

## Requirements

- Go 1.21+
- The coordination server must be publicly accessible (no NAT/firewall blocking inbound UDP on the server)
- Peers can be behind NAT/firewalls—that's the whole point

## How NAT Traversal Works

NATs typically block unsolicited inbound traffic but allow responses to outbound traffic. UDP hole punching exploits this:

1. Both peers send a UDP packet to each other's public address
2. Each NAT sees an outbound packet and creates a mapping
3. The peer's incoming packet now matches the mapping and is allowed through
4. Direct bidirectional communication is established

The key insight: both peers must punch at roughly the same time, using the same socket they registered with the coordination server.
