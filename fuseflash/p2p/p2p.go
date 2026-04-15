// Package p2p implements peer-to-peer networking for the fuseflash node,
// providing peer discovery, connection management, and message transport.
package p2p

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/dlema861011010/dlema861011010/fuseflash/base58"
)

// NodeID is a 32-byte identifier for a peer node.
type NodeID [32]byte

// String returns the hex-encoded form of the NodeID.
func (id NodeID) String() string {
	return hex.EncodeToString(id[:])
}

// Base58 returns the Base58-encoded form of the NodeID.
func (id NodeID) Base58() string {
	return base58.Encode(id[:])
}

// ParseNodeIDBase58 decodes a Base58 string into a NodeID.
func ParseNodeIDBase58(s string) (NodeID, error) {
	b, err := base58.DecodeFixed(s, 32)
	if err != nil {
		return NodeID{}, fmt.Errorf("parse node id base58: %w", err)
	}
	var id NodeID
	copy(id[:], b)
	return id, nil
}

// GenerateNodeID creates a cryptographically random NodeID.
func GenerateNodeID() (NodeID, error) {
	var id NodeID
	if _, err := rand.Read(id[:]); err != nil {
		return NodeID{}, fmt.Errorf("generate node id: %w", err)
	}
	return id, nil
}

// ParseNodeID decodes a hex string into a NodeID.
func ParseNodeID(s string) (NodeID, error) {
	b, err := hex.DecodeString(s)
	if err != nil {
		return NodeID{}, fmt.Errorf("parse node id: %w", err)
	}
	if len(b) != 32 {
		return NodeID{}, errors.New("parse node id: must be 32 bytes (64 hex chars)")
	}
	var id NodeID
	copy(id[:], b)
	return id, nil
}

// Peer represents a remote peer in the network.
type Peer struct {
	ID   NodeID
	Addr net.Addr
	conn net.Conn
	mu   sync.Mutex
}

// Send transmits a raw message to the peer.
func (p *Peer) Send(msg []byte) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.conn == nil {
		return errors.New("peer: not connected")
	}
	_, err := p.conn.Write(msg)
	return err
}

// Close shuts down the connection to the peer.
func (p *Peer) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.conn == nil {
		return nil
	}
	err := p.conn.Close()
	p.conn = nil
	return err
}

// MessageHandler is called whenever a message is received from a peer.
type MessageHandler func(peer *Peer, msg []byte)

// Server manages incoming and outgoing peer connections.
type Server struct {
	Self     NodeID
	listener net.Listener

	mu       sync.RWMutex
	peers    map[NodeID]*Peer
	handlers []MessageHandler

	quit chan struct{}
	wg   sync.WaitGroup
}

// NewServer creates a new P2P server with the given node identity.
func NewServer(id NodeID) *Server {
	return &Server{
		Self:  id,
		peers: make(map[NodeID]*Peer),
		quit:  make(chan struct{}),
	}
}

// AddHandler registers a MessageHandler that is invoked for every inbound
// message received from any peer.
func (s *Server) AddHandler(h MessageHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.handlers = append(s.handlers, h)
}

// Listen starts accepting connections on the given TCP address.
func (s *Server) Listen(addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("p2p listen: %w", err)
	}
	s.listener = ln
	s.wg.Add(1)
	go s.acceptLoop()
	return nil
}

// Connect dials a remote peer and registers it with the server.
func (s *Server) Connect(id NodeID, addr string) (*Peer, error) {
	conn, err := net.DialTimeout("tcp", addr, 10*time.Second)
	if err != nil {
		return nil, fmt.Errorf("p2p connect %s: %w", addr, err)
	}
	peer := &Peer{ID: id, Addr: conn.RemoteAddr(), conn: conn}
	s.addPeer(peer)
	s.wg.Add(1)
	go s.readLoop(peer)
	return peer, nil
}

// Peers returns a snapshot of all currently connected peers.
func (s *Server) Peers() []*Peer {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*Peer, 0, len(s.peers))
	for _, p := range s.peers {
		out = append(out, p)
	}
	return out
}

// PeerCount returns the number of connected peers.
func (s *Server) PeerCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.peers)
}

// Stop shuts down the server and all peer connections.
func (s *Server) Stop() {
	close(s.quit)
	if s.listener != nil {
		s.listener.Close()
	}
	s.mu.RLock()
	for _, p := range s.peers {
		p.Close()
	}
	s.mu.RUnlock()
	s.wg.Wait()
}

// Broadcast sends a message to every connected peer.
func (s *Server) Broadcast(msg []byte) {
	for _, p := range s.Peers() {
		p.Send(msg) //nolint:errcheck
	}
}

// addPeer registers a peer under the server lock.
func (s *Server) addPeer(p *Peer) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.peers[p.ID] = p
}

// removePeer unregisters a peer.
func (s *Server) removePeer(id NodeID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.peers, id)
}

// acceptLoop accepts incoming TCP connections until Stop is called.
func (s *Server) acceptLoop() {
	defer s.wg.Done()
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			select {
			case <-s.quit:
				return
			default:
			}
			continue
		}
		// Assign a temporary random NodeID for the accepted peer until a
		// handshake protocol is layered on top.
		id, err := GenerateNodeID()
		if err != nil {
			conn.Close()
			continue
		}
		peer := &Peer{ID: id, Addr: conn.RemoteAddr(), conn: conn}
		s.addPeer(peer)
		s.wg.Add(1)
		go s.readLoop(peer)
	}
}

// readLoop continuously reads messages from a peer until the connection is
// closed or the server is stopped.
func (s *Server) readLoop(peer *Peer) {
	defer s.wg.Done()
	defer func() {
		peer.Close()
		s.removePeer(peer.ID)
	}()

	buf := make([]byte, 65536)
	for {
		select {
		case <-s.quit:
			return
		default:
		}
		peer.mu.Lock()
		conn := peer.conn
		peer.mu.Unlock()
		if conn == nil {
			return
		}
		conn.SetReadDeadline(time.Now().Add(30 * time.Second)) //nolint:errcheck
		n, err := conn.Read(buf)
		if err != nil {
			return
		}
		msg := make([]byte, n)
		copy(msg, buf[:n])
		s.mu.RLock()
		handlers := s.handlers
		s.mu.RUnlock()
		for _, h := range handlers {
			h(peer, msg)
		}
	}
}
