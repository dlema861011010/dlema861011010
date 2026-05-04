package p2p_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/dlema861011010/dlema861011010/fuseflash/p2p"
)

func TestNodeCreation(t *testing.T) {
	n := p2p.NewNode("peer-1", "0xdeadbeef", "127.0.0.1:0")
	if n.PeerID != "peer-1" {
		t.Errorf("PeerID = %q, want peer-1", n.PeerID)
	}
	if n.Address != "0xdeadbeef" {
		t.Errorf("Address = %q, want 0xdeadbeef", n.Address)
	}
	if n.PeerCount() != 0 {
		t.Error("new node should have 0 peers")
	}
}

func TestPeerConnect(t *testing.T) {
	n1 := p2p.NewNode("peer-1", "0x1111", "127.0.0.1:19901")
	n2 := p2p.NewNode("peer-2", "0x2222", "127.0.0.1:19902")
	defer n1.Close()
	defer n2.Close()

	if err := n1.Listen(); err != nil {
		t.Fatalf("n1 listen: %v", err)
	}
	if err := n2.Listen(); err != nil {
		t.Fatalf("n2 listen: %v", err)
	}

	if err := n2.Connect("127.0.0.1:19901"); err != nil {
		t.Fatalf("n2.Connect: %v", err)
	}

	// Wait for handshake to complete.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if n1.PeerCount() == 1 && n2.PeerCount() == 1 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if n1.PeerCount() != 1 {
		t.Errorf("n1 peer count = %d, want 1", n1.PeerCount())
	}
}

func TestSendFlashTx(t *testing.T) {
	n1 := p2p.NewNode("peer-a", "0xaaaa", "127.0.0.1:19911")
	n2 := p2p.NewNode("peer-b", "0xbbbb", "127.0.0.1:19912")
	defer n1.Close()
	defer n2.Close()

	received := make(chan p2p.FlashTx, 1)
	n2.Handle(p2p.MsgFlashTx, func(msg *p2p.Message, from *p2p.Peer) {
		var tx p2p.FlashTx
		if err := json.Unmarshal(msg.Payload, &tx); err == nil {
			received <- tx
		}
	})

	if err := n1.Listen(); err != nil {
		t.Fatalf("n1 listen: %v", err)
	}
	if err := n2.Listen(); err != nil {
		t.Fatalf("n2 listen: %v", err)
	}
	if err := n1.Connect("127.0.0.1:19912"); err != nil {
		t.Fatalf("connect: %v", err)
	}

	// Wait for peers to be connected.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if n1.PeerCount() == 1 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	tx := p2p.FlashTx{
		From:   "0xaaaa",
		To:     "0xbbbb",
		Amount: "1000000000000000000",
		Nonce:  1,
		Sig:    "deadbeef",
	}
	if err := n1.SendFlashTx(tx); err != nil {
		t.Fatalf("SendFlashTx: %v", err)
	}

	select {
	case got := <-received:
		if got.Amount != tx.Amount {
			t.Errorf("amount = %q, want %q", got.Amount, tx.Amount)
		}
	case <-time.After(2 * time.Second):
		t.Error("timed out waiting for flash-tx")
	}
}
