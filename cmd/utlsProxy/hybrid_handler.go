package main

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"

	projlogger "crawler-platform/logger"
	"github.com/quic-go/quic-go"
)

// HybridHandler 混合模式处理器
// 同时支持CONNECT命令（TCP代理模式）和PACKET命令（TUN模式）
type HybridHandler struct {
	proxy     *UTLSProxy
	conn      *quic.Conn
	streams   map[quic.StreamID]*TCPProxyStream // CONNECT模式的流
	streamsMu sync.RWMutex
	tunnels   map[quic.StreamID]*IPTunnel       // PACKET模式的隧道
	tunnelsMu sync.RWMutex
	ctx       context.Context
	cancel    context.CancelFunc
}

// TCPProxyStream TCP代理流（用于CONNECT命令）
type TCPProxyStream struct {
	stream     *quic.Stream
	targetAddr net.Addr
	remoteConn net.Conn
	closeOnce  sync.Once
	readDone   chan struct{}
	writeDone  chan struct{}
}

// NewHybridHandler 创建混合模式处理器
func NewHybridHandler(proxy *UTLSProxy, conn *quic.Conn) *HybridHandler {
	ctx, cancel := context.WithCancel(context.Background())
	return &HybridHandler{
		proxy:   proxy,
		conn:    conn,
		streams: make(map[quic.StreamID]*TCPProxyStream),
		tunnels: make(map[quic.StreamID]*IPTunnel),
		ctx:     ctx,
		cancel:  cancel,
	}
}

// Start 启动混合处理器
func (h *HybridHandler) Start() {
	go h.acceptStreams()
}

// Stop 停止混合处理器
func (h *HybridHandler) Stop() {
	h.cancel()

	h.streamsMu.Lock()
	for _, stream := range h.streams {
		stream.Close()
	}
	h.streamsMu.Unlock()

	h.tunnelsMu.Lock()
	for _, tunnel := range h.tunnels {
		tunnel.Close()
	}
	h.tunnelsMu.Unlock()
}

// acceptStreams 接受QUIC流
func (h *HybridHandler) acceptStreams() {
	for {
		select {
		case <-h.ctx.Done():
			return
		default:
			stream, err := h.conn.AcceptStream(h.ctx)
			if err != nil {
				if h.ctx.Err() != nil {
					return
				}
				projlogger.Debug("接受QUIC流失败: %v", err)
				continue
			}

			// 处理每个流，自动识别模式
			go h.handleStream(stream)
		}
	}
}

// handleStream 处理单个流，自动识别CONNECT或PACKET模式
func (h *HybridHandler) handleStream(stream *quic.Stream) {
	defer stream.Close()

	streamID := stream.StreamID()

	// 读取第一个TUIC协议头来识别模式
	header := make([]byte, 4)
	if _, err := io.ReadFull(stream, header); err != nil {
		projlogger.Debug("读取TUIC协议头失败: %v", err)
		return
	}

	version := header[0]
	command := header[1]
	dataLen := binary.BigEndian.Uint16(header[2:4])

	if version != 5 {
		projlogger.Error("不支持的TUIC版本: %d", version)
		return
	}

	// 读取payload
	payload := make([]byte, dataLen)
	if _, err := io.ReadFull(stream, payload); err != nil {
		projlogger.Debug("读取payload失败: %v", err)
		return
	}

	// 根据命令类型选择处理方式
	if command == 0 {
		// CONNECT命令：TCP代理模式（向后兼容）
		h.handleConnectCommand(streamID, stream, payload)
	} else if command == 1 {
		// PACKET命令：真正的TUN模式（IP数据包）
		h.handlePacketCommand(streamID, stream, payload)
	} else {
		projlogger.Error("不支持的TUIC命令: %d", command)
	}
}

// handleConnectCommand 处理CONNECT命令（TCP代理模式）
func (h *HybridHandler) handleConnectCommand(streamID quic.StreamID, stream *quic.Stream, payload []byte) {
	// 解析目标地址
	if len(payload) < 4 {
		projlogger.Error("CONNECT命令payload太短")
		return
	}

	addrType := payload[0]
	addrLen := int(payload[1])

	if len(payload) < 2+addrLen+2 {
		projlogger.Error("CONNECT命令payload数据不完整")
		return
	}

	var targetAddr string
	if addrType == 0 {
		targetAddr = string(payload[2 : 2+addrLen])
	} else if addrType == 1 {
		if addrLen == 4 {
			targetAddr = net.IP(payload[2:6]).String()
		}
	} else if addrType == 2 {
		if addrLen == 16 {
			targetAddr = fmt.Sprintf("[%s]", net.IP(payload[2:18]).String())
		}
	}

	port := binary.BigEndian.Uint16(payload[2+addrLen : 2+addrLen+2])
	target := fmt.Sprintf("%s:%d", targetAddr, port)

	projlogger.Debug("CONNECT命令 (TCP代理模式): %s", target)

	// 建立TCP连接
	tcpConn, err := net.DialTimeout("tcp", target, h.proxy.config.PoolConfig.ConnTimeout)
	if err != nil {
		projlogger.Error("连接到目标服务器失败 %s: %v", target, err)
		errorResp := make([]byte, 4)
		errorResp[0] = 5
		errorResp[1] = 2 // 错误响应
		binary.BigEndian.PutUint16(errorResp[2:4], 0)
		stream.Write(errorResp)
		return
	}

	// 发送成功响应
	successResp := make([]byte, 4)
	successResp[0] = 5
	successResp[1] = 0 // CONNECT成功
	binary.BigEndian.PutUint16(successResp[2:4], 0)
	stream.Write(successResp)

	// 处理HTTP请求数据（在payload的剩余部分）
	httpData := payload[2+addrLen+2:]
	if len(httpData) > 0 {
		if _, err := tcpConn.Write(httpData); err != nil {
			projlogger.Debug("写入HTTP数据失败: %v", err)
			tcpConn.Close()
			return
		}
	}

	// 创建TCP代理流
	tcpStream := &TCPProxyStream{
		stream:     stream,
		targetAddr: tcpConn.RemoteAddr(),
		remoteConn: tcpConn,
		readDone:   make(chan struct{}),
		writeDone:  make(chan struct{}),
	}

	// 注册流
	h.streamsMu.Lock()
	h.streams[streamID] = tcpStream
	h.streamsMu.Unlock()

	// 启动双向数据转发
	go h.forwardRemoteToQuic(tcpStream)
	go h.forwardQuicToRemote(tcpStream)

	// 等待完成
	<-tcpStream.readDone
	<-tcpStream.writeDone

	// 清理流
	h.streamsMu.Lock()
	delete(h.streams, streamID)
	h.streamsMu.Unlock()
}

// handlePacketCommand 处理PACKET命令（TUN模式）
func (h *HybridHandler) handlePacketCommand(streamID quic.StreamID, stream *quic.Stream, firstPacket []byte) {
	// 使用IPTunnelHandler的逻辑处理
	// 这里简化，直接使用IP隧道处理器
	iptHandler := NewIPTunnelHandler(h.proxy, h.conn)
	
	// 创建IP隧道
	tunnel := &IPTunnel{
		stream:   stream,
		tcpConns: make(map[string]*TCPConnection),
		readDone: make(chan struct{}),
		writeDone: make(chan struct{}),
	}

	h.tunnelsMu.Lock()
	h.tunnels[streamID] = tunnel
	h.tunnelsMu.Unlock()

	// 处理第一个数据包
	if err := iptHandler.handleIPPacket(tunnel, firstPacket); err != nil {
		projlogger.Debug("处理第一个IP数据包失败: %v", err)
	}

	// 继续处理后续数据包
	go iptHandler.processIncomingIPPackets(tunnel)
	go iptHandler.processOutgoingResponses(tunnel)

	<-tunnel.readDone
	<-tunnel.writeDone

	h.tunnelsMu.Lock()
	delete(h.tunnels, streamID)
	h.tunnelsMu.Unlock()
}

// forwardRemoteToQuic 从远程连接读取并转发到QUIC流
func (h *HybridHandler) forwardRemoteToQuic(stream *TCPProxyStream) {
	defer func() {
		close(stream.readDone)
		stream.Close()
	}()

	buffer := make([]byte, 32*1024)
	for {
		select {
		case <-h.ctx.Done():
			return
		default:
			n, err := stream.remoteConn.Read(buffer)
			if n > 0 {
				// 构建TUIC PACKET响应
				packet := make([]byte, 4+n)
				packet[0] = 5
				packet[1] = 1 // PACKET命令
				binary.BigEndian.PutUint16(packet[2:4], uint16(n))
				copy(packet[4:], buffer[:n])

				if _, err := stream.stream.Write(packet); err != nil {
					projlogger.Debug("写入QUIC流失败: %v", err)
					return
				}
			}
			if err != nil {
				if err != io.EOF {
					projlogger.Debug("读取远程连接失败: %v", err)
				}
				return
			}
		}
	}
}

// forwardQuicToRemote 从QUIC流读取并转发到远程连接
func (h *HybridHandler) forwardQuicToRemote(stream *TCPProxyStream) {
	defer func() {
		close(stream.writeDone)
		stream.Close()
	}()

	buffer := make([]byte, 32*1024)
	for {
		select {
		case <-h.ctx.Done():
			return
		default:
			// 读取TUIC协议头
			header := make([]byte, 4)
			if _, err := io.ReadFull(stream.stream, header); err != nil {
				if err != io.EOF {
					projlogger.Debug("读取QUIC流头失败: %v", err)
				}
				return
			}

			cmd := header[1]
			dataLen := binary.BigEndian.Uint16(header[2:4])

			if cmd == 1 { // PACKET命令
				if dataLen > uint16(len(buffer)) {
					projlogger.Debug("数据包太大: %d", dataLen)
					return
				}

				data := buffer[:dataLen]
				if _, err := io.ReadFull(stream.stream, data); err != nil {
					projlogger.Debug("读取数据包失败: %v", err)
					return
				}

				if _, err := stream.remoteConn.Write(data); err != nil {
					projlogger.Debug("写入远程连接失败: %v", err)
					return
				}
			}
		}
	}
}

// Close 关闭TCP代理流
func (s *TCPProxyStream) Close() {
	s.closeOnce.Do(func() {
		if s.remoteConn != nil {
			s.remoteConn.Close()
		}
		if s.stream != nil {
			s.stream.Close()
		}
	})
}
