package p2p

import (
	"encoding/hex"
	"net"
	"testing"
	"time"
)

func TestGenerateNodeID(t *testing.T) {
	id1, err := GenerateNodeID()
	if err != nil {
		t.Fatalf("GenerateNodeID: %v", err)
	}
	id2, err := GenerateNodeID()
	if err != nil {
		t.Fatalf("GenerateNodeID: %v", err)
	}
	if id1 == id2 {
		t.Error("expected distinct node IDs, got duplicates")
	}
}

func TestParseNodeID_RoundTrip(t *testing.T) {
	id, err := GenerateNodeID()
	if err != nil {
		t.Fatalf("GenerateNodeID: %v", err)
	}
	parsed, err := ParseNodeID(id.String())
	if err != nil {
		t.Fatalf("ParseNodeID: %v", err)
	}
	if id != parsed {
		t.Errorf("round-trip mismatch: want %s, got %s", id, parsed)
	}
}

func TestParseNodeID_InvalidHex(t *testing.T) {
	_, err := ParseNodeID("zzzz")
	if err == nil {
		t.Error("expected error for invalid hex, got nil")
	}
}

func TestParseNodeID_WrongLength(t *testing.T) {
	_, err := ParseNodeID(hex.EncodeToString([]byte("short")))
	if err == nil {
		t.Error("expected error for wrong length, got nil")
	}
}

func TestServerPeerCount(t *testing.T) {
	id, _ := GenerateNodeID()
	srv := NewServer(id)
	if srv.PeerCount() != 0 {
		t.Errorf("expected 0 peers, got %d", srv.PeerCount())
	}
}

func TestServerListenConnect(t *testing.T) {
	serverID, _ := GenerateNodeID()
	srv := NewServer(serverID)
	if err := srv.Listen("127.0.0.1:0"); err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer srv.Stop()

	addr := srv.listener.Addr().String()

	clientID, _ := GenerateNodeID()
	client := NewServer(clientID)
	defer client.Stop()

	received := make(chan []byte, 1)
	srv.AddHandler(func(_ *Peer, msg []byte) {
		cp := make([]byte, len(msg))
		copy(cp, msg)
		select {
		case received <- cp:
		default:
		}
	})

	peerID, _ := GenerateNodeID()
	peer, err := client.Connect(peerID, addr)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}

	want := []byte("hello fuseflash")
	if err := peer.Send(want); err != nil {
		t.Fatalf("Send: %v", err)
	}

	select {
	case got := <-received:
		if string(got) != string(want) {
			t.Errorf("got %q, want %q", got, want)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for message")
	}
}

func TestServerBroadcast(t *testing.T) {
	serverID, _ := GenerateNodeID()
	srv := NewServer(serverID)
	if err := srv.Listen("127.0.0.1:0"); err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer srv.Stop()

	addr := srv.listener.Addr().String()

	received := make(chan struct{}, 2)
	srv.AddHandler(func(_ *Peer, _ []byte) {
		select {
		case received <- struct{}{}:
		default:
		}
	})

	for i := 0; i < 2; i++ {
		c, err := net.Dial("tcp", addr)
		if err != nil {
			t.Fatalf("dial %d: %v", i, err)
		}
		defer c.Close()
	}

	// Allow the server to register both peers.
	time.Sleep(100 * time.Millisecond)

	if srv.PeerCount() != 2 {
		t.Fatalf("expected 2 peers, got %d", srv.PeerCount())
	}
}

func TestPeerSendDisconnected(t *testing.T) {
	p := &Peer{}
	err := p.Send([]byte("test"))
	if err == nil {
		t.Error("expected error sending to disconnected peer")
	}
}
