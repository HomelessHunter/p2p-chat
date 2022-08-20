package node

import (
	"bufio"
	"context"
	"fmt"
	"testing"

	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/protocol"
	"github.com/stretchr/testify/require"
)

func TestNode(t *testing.T) {
	node1, addr1 := testNewNode(t, context.TODO())
	defer node1.Close()

	node2, _ := testNewNode(t, context.TODO())
	defer node2.Close()

	// t.Run("Chat", func(t *testing.T) {
	// 	node1.SetupStreamHandlers(map[protocol.ID]network.StreamHandler{
	// 		"/chat/1.0.0": testGetChatHandler(t, ctx),
	// 	})
	// 	peerID := testEstablishConn(t, ctx, *node2, addr1)
	// 	node2.testOpenNewStream(t, ctx, peerID, "/chat/1.0.0")
	// })

	node1.SetupStreamHandlers(map[protocol.ID]network.StreamHandler{
		"/chat/1.0.0": testGetChatHandler(t, context.TODO()),
	})
	peerID := testEstablishConn(t, context.TODO(), *node2, addr1)
	node2.testOpenNewStream(t, context.TODO(), peerID, "/chat/1.0.0")
}

func testNewNode(t *testing.T, ctx context.Context) (*Node, peer.AddrInfo) {
	node, err := NewNode(ctx, 0, "private_key")
	require.NoError(t, err)
	addr, err := node.NodeAddrs()
	require.NoError(t, err)
	return node, *addr[0]
}

func testEstablishConn(t *testing.T, ctx context.Context, node Node, addr peer.AddrInfo) peer.ID {
	peerID, err := node.EstablishNewConn(ctx, fmt.Sprintf("%s/p2p/%s", addr.Addrs[0], addr.ID))
	require.NoError(t, err)
	return peerID
}

func testReadChat(t *testing.T, ctx context.Context, reader *bufio.Reader) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			text, err := reader.ReadString('\n')
			// if err == io.EOF {
			// } else {
			// 	require.NoError(t, err)
			// }
			require.NoError(t, err)
			require.Equal(t, "Hello\n", text)
			return
		}
	}
}

func testWriteChat(t *testing.T, ctx context.Context, writer *bufio.Writer) {
	for {
		select {
		case <-ctx.Done():
			fmt.Println("ctx canceled")
			return
		default:
			_, err := writer.WriteString("Hello\n")
			require.NoError(t, err)
			writer.Flush()
			return
		}
	}
}

func testGetChatHandler(t *testing.T, ctx context.Context) func(network.Stream) {
	return func(s network.Stream) {
		rw := bufio.NewReadWriter(bufio.NewReaderSize(s, 512), bufio.NewWriterSize(s, 512))
		defer s.Close()

		testReadChat(t, ctx, rw.Reader)
		// go testWriteChat(t, ctx, rw.Writer)

		s.Write([]byte("Peer closed the connection"))
		s.Reset()
	}
}

func (n *Node) testOpenNewStream(t *testing.T, ctx context.Context, peerID peer.ID, pID ...protocol.ID) {
	s, err := n.h.NewStream(ctx, peerID, pID...)
	require.NoError(t, err)
	defer s.Close()

	rw := bufio.NewReadWriter(bufio.NewReaderSize(s, 512), bufio.NewWriterSize(s, 512))

	// go testReadChat(t, ctx, rw.Reader)
	testWriteChat(t, ctx, rw.Writer)

	return
}
