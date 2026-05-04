// Command node runs a FuseFlash P2P node on the FuseSpark testnet.
//
// Usage:
//
//	node -listen :7777 -peers peer1:7777,peer2:7777
//
// Environment variables:
//
//	FUSEFLASH_PRIVATE_KEY   hex-encoded 32-byte private key (generated if absent)
//	FUSEFLASH_LISTEN        listen address (default :7777)
//	FUSEFLASH_PEERS         comma-separated peer addresses to dial on startup
package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/dlema861011010/dlema861011010/fuseflash/p2p"
	"github.com/dlema861011010/dlema861011010/fuseflash/wallet"
)

func main() {
	listen := flag.String("listen", env("FUSEFLASH_LISTEN", ":7777"), "listen address")
	peers := flag.String("peers", env("FUSEFLASH_PEERS", ""), "comma-separated peer addresses")
	flag.Parse()

	// Load or generate wallet.
	var w *wallet.Wallet
	var err error
	if key := os.Getenv("FUSEFLASH_PRIVATE_KEY"); key != "" {
		w, err = wallet.FromPrivateKeyHex(key)
	} else {
		w, err = wallet.Generate()
	}
	if err != nil {
		log.Fatalf("wallet init: %v", err)
	}
	log.Printf("[fuseflash] wallet address : %s", w.Address)
	log.Printf("[fuseflash] peer ID        : %s", w.PeerID)
	log.Printf("[fuseflash] chain          : %s (chain ID %d)", wallet.ChainName, wallet.ChainID)
	log.Printf("[fuseflash] explorer       : %s", wallet.ExplorerURL)

	node := p2p.NewNode(w.PeerID, w.Address, *listen)
	if err := node.Listen(); err != nil {
		log.Fatalf("listen: %v", err)
	}

	// Dial bootstrap peers.
	if *peers != "" {
		for _, addr := range strings.Split(*peers, ",") {
			addr = strings.TrimSpace(addr)
			if addr == "" {
				continue
			}
			log.Printf("[fuseflash] dialing peer %s", addr)
			if err := node.Connect(addr); err != nil {
				log.Printf("[fuseflash] peer %s: %v", addr, err)
			}
		}
	}

	// Block until SIGINT/SIGTERM.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	log.Println("[fuseflash] shutting down")
	node.Close()
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
