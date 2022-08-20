package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/HomelessHunter/p2p-chat/client/internal/tui"
	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	// defer cancel()

	port := flag.Int("p", 0, "Port to listen on")
	// dest := flag.String("d", "", "Destination string")
	pnKey := flag.String("key", "", "Key for private network")

	flag.Parse()

	p := tea.NewProgram(tui.InitialModel(ctx, cancel, *port, *pnKey), tea.WithAltScreen())
	if err := p.Start(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	// sig := make(chan os.Signal, 1)
	// signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)
	// <-sig
}
