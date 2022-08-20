package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/p2p/protocol/circuitv2/relay"
)

func main() {
	h, err := libp2p.New(
		libp2p.DisableRelay(),
	)
	if err != nil {
		panic(err)
	}
	defer h.Close()

	r, err := relay.New(h, relay.WithResources(relay.Resources{}))
	if err != nil {
		panic(err)
	}
	defer r.Close()
	fmt.Println(h.Addrs())

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)
	<-sig
}
