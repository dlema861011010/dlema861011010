// Package p2p implements the FuseFlash Peer-to-peer protocol over TCP.
//
// Each node listens for incoming Peer connections, exchanges a handshake
// that includes the sender's PeerID and wallet address, and then processes
// FuseFlash messages (ping, flash-tx, route).
package p2p

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"time"
)

// MsgType identifies the kind of FuseFlash message.
type MsgType string

const (
	MsgPing    MsgType = "ping"
	MsgPong    MsgType = "pong"
	MsgFlashTx MsgType = "flash-tx"
	MsgRoute   MsgType = "route"
	MsgHandshake MsgType = "handshake"

	handshakeTimeout = 10 * time.Second
	dialTimeout      = 15 * time.Second
)

// Message is the wire format for all FuseFlash P2P messages (newline-delimited JSON).
type Message struct {
	Type      MsgType         `json:"type"`
	From      string          `json:"from"`       // PeerID of sender
	To        string          `json:"to"`         // PeerID of recipient (empty = broadcast)
	Payload   json.RawMessage `json:"payload"`    // type-specific data
	Timestamp int64           `json:"timestamp"`  // unix millis
}

// Handshake is sent on connection open to identify the Peer.
type Handshake struct {
	PeerID  string `json:"peer_id"`
	Address string `json:"address"` // EVM wallet address
	Version string `json:"version"`
}

// FlashTx represents a fast P2P micro-transaction routed off-chain.
type FlashTx struct {
	From   string `json:"from"`   // EVM sender address
	To     string `json:"to"`     // EVM recipient address
	Amount string `json:"amount"` // value in wei (string to avoid overflow)
	Nonce  uint64 `json:"nonce"`
	Sig    string `json:"sig"` // 64-byte hex ECDSA signature
}

// Peer represents an active remote Peer connection.
type Peer struct {
	conn    net.Conn
	peerID  string
	address string
	enc     *json.Encoder
	mu      sync.Mutex
}

func (p *Peer) send(msg *Message) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.enc.Encode(msg)
}

// Node is a FuseFlash P2P node.
type Node struct {
	PeerID  string
	Address string // wallet address

	listenAddr string
	peers      map[string]*Peer
	mu         sync.RWMutex
	handlers   map[MsgType]HandlerFunc
	quit       chan struct{}
}

// HandlerFunc processes an inbound message from a remote Peer.
type HandlerFunc func(msg *Message, from *Peer)

// NewNode creates a FuseFlash node with the given identity and listen address.
func NewNode(peerID, walletAddress, listenAddr string) *Node {
	n := &Node{
		PeerID:     peerID,
		Address:    walletAddress,
		listenAddr: listenAddr,
		peers:      make(map[string]*Peer),
		handlers:   make(map[MsgType]HandlerFunc),
		quit:       make(chan struct{}),
	}
	n.Handle(MsgPing, n.handlePing)
	n.Handle(MsgFlashTx, n.handleFlashTx)
	return n
}

// Handle registers a handler for the given message type.
func (n *Node) Handle(t MsgType, h HandlerFunc) {
	n.handlers[t] = h
}

// Listen starts accepting inbound Peer connections.
func (n *Node) Listen() error {
	ln, err := net.Listen("tcp", n.listenAddr)
	if err != nil {
		return fmt.Errorf("p2p: listen %s: %w", n.listenAddr, err)
	}
	log.Printf("[fuseflash] node %s listening on %s", n.PeerID, n.listenAddr)
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				select {
				case <-n.quit:
					return
				default:
					log.Printf("[fuseflash] accept error: %v", err)
					continue
				}
			}
			go n.handleConn(conn)
		}
	}()
	return nil
}

// Connect dials a remote Peer and performs the FuseFlash handshake.
func (n *Node) Connect(addr string) error {
	conn, err := net.DialTimeout("tcp", addr, dialTimeout)
	if err != nil {
		return fmt.Errorf("p2p: dial %s: %w", addr, err)
	}
	go n.handleConn(conn)
	return nil
}

// Broadcast sends a message to all connected peers.
func (n *Node) Broadcast(msg *Message) {
	n.mu.RLock()
	defer n.mu.RUnlock()
	for _, p := range n.peers {
		if err := p.send(msg); err != nil {
			log.Printf("[fuseflash] broadcast to %s failed: %v", p.peerID, err)
		}
	}
}

// SendFlashTx broadcasts a FlashTx to all peers.
func (n *Node) SendFlashTx(tx FlashTx) error {
	payload, err := json.Marshal(tx)
	if err != nil {
		return err
	}
	n.Broadcast(&Message{
		Type:      MsgFlashTx,
		From:      n.PeerID,
		Payload:   payload,
		Timestamp: time.Now().UnixMilli(),
	})
	return nil
}

// PeerCount returns the number of connected peers.
func (n *Node) PeerCount() int {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return len(n.peers)
}

// Close shuts the node down.
func (n *Node) Close() {
	close(n.quit)
	n.mu.Lock()
	defer n.mu.Unlock()
	for _, p := range n.peers {
		p.conn.Close()
	}
}

// --- internal ---

func (n *Node) handleConn(conn net.Conn) {
	dec := json.NewDecoder(bufio.NewReader(conn))
	enc := json.NewEncoder(conn)

	// Send our handshake.
	hs := Handshake{PeerID: n.PeerID, Address: n.Address, Version: "1.0.0"}
	payload, _ := json.Marshal(hs)
	_ = enc.Encode(&Message{
		Type:      MsgHandshake,
		From:      n.PeerID,
		Payload:   payload,
		Timestamp: time.Now().UnixMilli(),
	})

	// Read remote handshake.
	conn.SetDeadline(time.Now().Add(handshakeTimeout))
	var msg Message
	if err := dec.Decode(&msg); err != nil {
		log.Printf("[fuseflash] handshake read error from %s: %v", conn.RemoteAddr(), err)
		conn.Close()
		return
	}
	conn.SetDeadline(time.Time{})

	var remoteHS Handshake
	if msg.Type != MsgHandshake || json.Unmarshal(msg.Payload, &remoteHS) != nil {
		log.Printf("[fuseflash] invalid handshake from %s", conn.RemoteAddr())
		conn.Close()
		return
	}

	p := &Peer{
		conn:    conn,
		peerID:  remoteHS.PeerID,
		address: remoteHS.Address,
		enc:     enc,
	}
	n.addPeer(p)
	defer n.removePeer(p)

	log.Printf("[fuseflash] Peer connected: %s (%s)", p.peerID, p.address)

	for {
		var m Message
		if err := dec.Decode(&m); err != nil {
			if err != io.EOF {
				log.Printf("[fuseflash] read from %s: %v", p.peerID, err)
			}
			return
		}
		n.dispatch(&m, p)
	}
}

func (n *Node) dispatch(msg *Message, from *Peer) {
	if h, ok := n.handlers[msg.Type]; ok {
		h(msg, from)
	}
}

func (n *Node) addPeer(p *Peer) {
	n.mu.Lock()
	n.peers[p.peerID] = p
	n.mu.Unlock()
}

func (n *Node) removePeer(p *Peer) {
	n.mu.Lock()
	delete(n.peers, p.peerID)
	n.mu.Unlock()
	p.conn.Close()
	log.Printf("[fuseflash] Peer disconnected: %s", p.peerID)
}

func (n *Node) handlePing(msg *Message, from *Peer) {
	_ = from.send(&Message{
		Type:      MsgPong,
		From:      n.PeerID,
		To:        from.peerID,
		Timestamp: time.Now().UnixMilli(),
	})
}

func (n *Node) handleFlashTx(msg *Message, from *Peer) {
	var tx FlashTx
	if err := json.Unmarshal(msg.Payload, &tx); err != nil {
		log.Printf("[fuseflash] bad flash-tx from %s: %v", from.peerID, err)
		return
	}
	log.Printf("[fuseflash] flash-tx %s -> %s amount=%s nonce=%d",
		tx.From, tx.To, tx.Amount, tx.Nonce)
}
