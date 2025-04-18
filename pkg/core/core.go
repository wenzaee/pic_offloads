package core

import (
	"context"
	"fmt"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"os"
)

var Edgehost host.Host

func RequestHostname(ctx context.Context, host host.Host, peerID peer.ID) (string, error) {
	// 创建一个新的流
	stream, err := host.NewStream(ctx, peerID, "/hostname-protocol")
	if err != nil {
		return "", fmt.Errorf("failed to create new stream: %w", err)
	}
	defer stream.Close()

	// 发送请求数据
	_, err = stream.Write([]byte("request-hostname"))
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}

	// 读取响应数据（主机名）
	buf := make([]byte, 256)
	n, err := stream.Read(buf)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	// 返回对方的主机名
	return string(buf[:n]), nil
}
func HandleRequest(s network.Stream) {
	defer s.Close()

	// 读取请求数据
	buf := make([]byte, 256)
	_, err := s.Read(buf)
	if err != nil {
		fmt.Println("Error reading request:", err)
		return
	}

	// 获取当前主机名
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}

	// 发送主机名作为响应
	_, err = s.Write([]byte(hostname))
	if err != nil {
		fmt.Println("Error sending hostname:", err)
	}
}
