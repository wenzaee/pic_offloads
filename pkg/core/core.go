package core

import (
	"context"
	"fmt"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"os"
	deafault "pic_offload/pkg/apis"
)

var Edgehost host.Host

func RequestHostname(ctx context.Context, host host.Host, peerID peer.ID) (string, error) {
	stream, err := host.NewStream(ctx, peerID, deafault.HostnameProtocol)
	if err != nil {
		return "", fmt.Errorf("failed to create new stream: %w", err)
	}
	defer stream.Close()

	_, err = stream.Write([]byte("request-hostname"))
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}

	buf := make([]byte, 256)
	n, err := stream.Read(buf)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	return string(buf[:n]), nil
}
func HandleRequest(s network.Stream) {
	defer s.Close()

	buf := make([]byte, 256)
	_, err := s.Read(buf)
	if err != nil {
		fmt.Println("Error reading request:", err)
		return
	}

	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}

	_, err = s.Write([]byte(hostname))
	if err != nil {
		fmt.Println("Error sending hostname:", err)
	}
}
