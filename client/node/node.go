package node

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/pnet"
	"github.com/libp2p/go-libp2p-core/protocol"
	noise "github.com/libp2p/go-libp2p-noise"
	libp2ptls "github.com/libp2p/go-libp2p-tls"
	"github.com/libp2p/go-libp2p/config"
	"github.com/libp2p/go-libp2p/p2p/host/autorelay"
	"github.com/libp2p/go-libp2p/p2p/net/connmgr"
	"github.com/libp2p/go-tcp-transport"
)

const (
	CHATPID string = "/chat/1.0.0"
)

type Node struct {
	h host.Host
}

func NewNodeDefault(configs ...config.Option) (*Node, error) {
	host, err := libp2p.New(configs...)
	return &Node{host}, err
}

func NewNode(ctx context.Context, port int, pnKey string) (*Node, error) {
	transport := libp2p.ChainOptions(
		libp2p.Transport(tcp.NewTCPTransport),
	)

	priv, _, err := crypto.GenerateKeyPair(crypto.Ed25519, -1)

	listenAddrs := libp2p.ListenAddrStrings(
		fmt.Sprintf("/ip4/0.0.0.0/tcp/%d", port),
	)

	security := libp2p.ChainOptions(
		libp2p.Security(libp2ptls.ID, libp2ptls.New),
		libp2p.Security(noise.ID, noise.New),
	)

	connMngr, err := connmgr.NewConnManager(
		1,
		2,
		connmgr.WithGracePeriod(time.Minute),
	)
	if err != nil {
		return nil, fmt.Errorf("NewNode: %v", err)
	}

	var privateNet config.Option

	if pnKey != "" {
		keyHash := sha256.New()
		_, err = keyHash.Write([]byte(pnKey))
		if err != nil {
			return nil, fmt.Errorf("NewNode: %v", err)
		}
		s := fmt.Sprintf("/key/swarm/psk/1.0.0/\n/base64/\n%s", hex.EncodeToString(keyHash.Sum(nil)))
		pskey, err := pnet.DecodeV1PSK(strings.NewReader(s))
		if err != nil {
			if err != io.EOF {
				return nil, fmt.Errorf("NewNode: %v", err)
			}
		}
		privateNet = libp2p.PrivateNetwork(pskey)
	}
	n, err := NewNodeDefault(
		transport, listenAddrs, security, privateNet,
		libp2p.Identity(priv),
		libp2p.ConnectionManager(connMngr),
		libp2p.NATPortMap(),
		libp2p.EnableAutoRelay(
			autorelay.WithBackoff(15*time.Second),
			autorelay.WithMaxAttempts(2),
		),
	)

	// n.SetDefaultHandlers(ctx, textReadCh, textWriteCh)

	return n, err
}

func (n *Node) SetDefaultHandlers(ctx context.Context, textReadCh, textWriteCh chan []byte) {
	n.h.SetStreamHandler(protocol.ID(CHATPID), getChatHandler(ctx, textReadCh, textWriteCh))
}

func (n *Node) DeleteHandler(pID string) {
	n.h.RemoveStreamHandler(protocol.ID(pID))
}

func (n *Node) SetupStreamHandlers(handlers map[protocol.ID]network.StreamHandler) {
	for pid, handler := range handlers {
		n.h.SetStreamHandler(pid, handler)
	}
}

func (n *Node) NodeAddrs() ([]*peer.AddrInfo, error) {
	var err error
	addrs := make([]*peer.AddrInfo, 2)
	for i, addr := range n.h.Addrs() {
		if i >= len(addrs) {
			break
		}
		addrs[i], err = peer.AddrInfoFromString(fmt.Sprintf("%v/p2p/%v", addr, n.h.ID()))
		if err != nil {
			return nil, fmt.Errorf("NodeAddrs: %v", err)
		}
	}
	return addrs, nil
}

func (n *Node) Close() error {
	return n.h.Close()
}

func getChatHandler(ctx context.Context, textReadCh, msgWriteCh chan []byte) func(network.Stream) {
	return func(s network.Stream) {
		rw := bufio.NewReadWriter(bufio.NewReaderSize(s, 512), bufio.NewWriterSize(s, 512))
		defer s.Reset()

		go func(ctx context.Context, reader *bufio.Reader, textReadCh chan []byte) {
			err := readChat(ctx, rw.Reader, textReadCh)
			if err != nil {
				panic(err)
			}
		}(ctx, rw.Reader, textReadCh)
		go func(ctx context.Context, reader *bufio.Writer, textWriteCh chan []byte) {
			err := writeChat(ctx, rw.Writer, msgWriteCh)
			if err != nil {
				panic(err)
			}
		}(ctx, rw.Writer, msgWriteCh)

		<-ctx.Done()
		s.Write([]byte("Peer closed the connection"))
	}
}

func readChat(ctx context.Context, reader *bufio.Reader, msgReadCh chan<- []byte) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
			msg, err := reader.ReadBytes('\n')
			if err != nil {
				return fmt.Errorf("readChat: %v", err)
			}
			msgReadCh <- msg
		}
	}
}

func writeChat(ctx context.Context, writer *bufio.Writer, msgWriteCh <-chan []byte) error {
	// reader := bufio.NewReaderSize(os.Stdin, 512)
	for {
		select {
		case <-ctx.Done():
			return nil
		case msg := <-msgWriteCh:
			// text, err := reader.ReadString('\n')
			// if err != nil {
			// 	return fmt.Errorf("writeChat: %v", err)
			// }
			_, err := writer.WriteString(fmt.Sprintf("%s", msg))
			if err != nil {
				return fmt.Errorf("writeChat: %v", err)
			}
			writer.Flush()
		}
	}
}

func (n *Node) EstablishNewConn(ctx context.Context, dest string) (peer.ID, error) {
	peerAddr, err := peer.AddrInfoFromString(dest)
	if err != nil {
		return "", fmt.Errorf("establishNewConn -> AddrInfroFromString: %v", err)
	}
	err = n.h.Connect(ctx, *peerAddr)
	if err != nil {
		return "", fmt.Errorf("establishNewConn -> Connect: %v", err)
	}

	return peerAddr.ID, nil
}

func (n *Node) OpenNewStream(ctx context.Context, msgReadCh, msgWriteCh chan []byte, peerID peer.ID, pID ...protocol.ID) error {
	s, err := n.h.NewStream(ctx, peerID, pID...)
	if err != nil {
		return fmt.Errorf("openNewStream -> NewStream: %v", err)
	}
	defer s.Reset()

	rw := bufio.NewReadWriter(bufio.NewReaderSize(s, 512), bufio.NewWriterSize(s, 512))

	go func(ctx context.Context, reader *bufio.Reader, textReadCh chan []byte) {
		err := readChat(ctx, rw.Reader, textReadCh)
		if err != nil {
			panic(err)
		}
	}(ctx, rw.Reader, msgReadCh)
	go func(ctx context.Context, reader *bufio.Writer, textWriteCh chan []byte) {
		err := writeChat(ctx, rw.Writer, msgWriteCh)
		if err != nil {
			panic(err)
		}
	}(ctx, rw.Writer, msgWriteCh)

	<-ctx.Done()
	return nil
}
