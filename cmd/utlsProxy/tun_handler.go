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

// TUNHandler TUN层处理器
// 负责处理TUIC协议中的TUN层数据包（IP数据包）
type TUNHandler struct {
	proxy     *UTLSProxy
	conn      *quic.Conn
	streams   map[quic.StreamID]*TUNStream
	streamsMu sync.RWMutex
	ctx       context.Context
	cancel    context.CancelFunc
}

// TUNStream TUN流，每个流对应一个目标连接
type TUNStream struct {
	stream     *quic.Stream
	targetAddr net.Addr
	remoteConn net.Conn // 到目标服务器的TCP连接
	closeOnce  sync.Once
	readDone   chan struct{}
	writeDone  chan struct{}
}

// NewTUNHandler 创建新的TUN处理器
func NewTUNHandler(proxy *UTLSProxy, conn *quic.Conn) *TUNHandler {
	ctx, cancel := context.WithCancel(context.Background())
	return &TUNHandler{
		proxy:   proxy,
		conn:    conn,
		streams: make(map[quic.StreamID]*TUNStream),
		ctx:     ctx,
		cancel:  cancel,
	}
}

// Start 启动TUN处理器
func (h *TUNHandler) Start() {
	// 启动接受流的goroutine
	go h.acceptStreams()
}

// Stop 停止TUN处理器
func (h *TUNHandler) Stop() {
	h.cancel()

	// 关闭所有流
	h.streamsMu.Lock()
	for _, stream := range h.streams {
		stream.Close()
	}
	h.streamsMu.Unlock()
}

// acceptStreams 接受QUIC流
func (h *TUNHandler) acceptStreams() {
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

			// 处理每个流
			go h.handleStream(stream)
		}
	}
}

// handleStream 处理单个QUIC流
func (h *TUNHandler) handleStream(stream *quic.Stream) {
	defer stream.Close()

	streamID := stream.StreamID()

	// 读取TUIC协议头
	header := make([]byte, 4)
	if _, err := io.ReadFull(stream, header); err != nil {
		projlogger.Debug("读取TUIC协议头失败: %v", err)
		return
	}

	// 解析TUIC协议头: [版本(1字节)][命令(1字节)][长度(2字节)]
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

	// 处理不同的命令
	switch command {
	case 0: // CONNECT - 建立到目标服务器的连接
		h.handleConnect(streamID, stream, payload)
	case 1: // PACKET - 转发数据包
		h.handlePacket(streamID, payload)
	default:
		projlogger.Error("不支持的TUIC命令: %d", command)
	}
}

// handleConnect 处理CONNECT命令，建立到目标服务器的连接
// TUIC协议中，CONNECT命令用于建立TCP连接，后续数据通过PACKET命令传输
func (h *TUNHandler) handleConnect(streamID quic.StreamID, stream *quic.Stream, payload []byte) {
	// 解析目标地址
	// payload格式: [地址类型(1字节)][地址长度(1字节)][地址][端口(2字节)]
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
		// 域名
		targetAddr = string(payload[2 : 2+addrLen])
	} else if addrType == 1 {
		// IPv4
		if addrLen == 4 {
			targetAddr = net.IP(payload[2:6]).String()
		}
	} else if addrType == 2 {
		// IPv6
		if addrLen == 16 {
			targetAddr = fmt.Sprintf("[%s]", net.IP(payload[2:18]).String())
		}
	}

	port := binary.BigEndian.Uint16(payload[2+addrLen : 2+addrLen+2])
	target := fmt.Sprintf("%s:%d", targetAddr, port)

	projlogger.Debug("TUN CONNECT: %s (stream %d)", target, streamID)

	// 建立到目标服务器的TCP连接
	tcpConn, err := net.DialTimeout("tcp", target, h.proxy.config.PoolConfig.ConnTimeout)
	if err != nil {
		projlogger.Error("连接到目标服务器失败 %s: %v", target, err)
		// 发送错误响应
		errorResp := make([]byte, 4)
		errorResp[0] = 5 // 版本
		errorResp[1] = 2 // 错误响应
		binary.BigEndian.PutUint16(errorResp[2:4], 0)
		stream.Write(errorResp)
		return
	}

	// 创建TUN流
	tunStream := &TUNStream{
		stream:     stream,
		targetAddr: tcpConn.RemoteAddr(),
		remoteConn: tcpConn,
		readDone:   make(chan struct{}),
		writeDone:  make(chan struct{}),
	}

	// 注册流
	h.streamsMu.Lock()
	h.streams[streamID] = tunStream
	h.streamsMu.Unlock()

	// 发送成功响应
	successResp := make([]byte, 4)
	successResp[0] = 5 // 版本
	successResp[1] = 0 // CONNECT成功
	binary.BigEndian.PutUint16(successResp[2:4], 0)
	stream.Write(successResp)

	// 启动双向数据转发
	go h.forwardRemoteToQuic(tunStream)
	go h.forwardQuicToRemote(tunStream)

	// 等待完成
	<-tunStream.readDone
	<-tunStream.writeDone

	// 清理流
	h.streamsMu.Lock()
	delete(h.streams, streamID)
	h.streamsMu.Unlock()
}

// handlePacket 处理PACKET命令，转发TCP数据包（不是IP数据包，而是TCP层数据）
// 注意：虽然叫TUN，但TUIC协议实际上是TCP代理，不是真正的IP层隧道
func (h *TUNHandler) handlePacket(streamID quic.StreamID, payload []byte) {
	h.streamsMu.RLock()
	tunStream, exists := h.streams[streamID]
	h.streamsMu.RUnlock()

	if !exists {
		projlogger.Debug("流不存在，可能已关闭: %d", streamID)
		return
	}

	// 将TCP数据包转发到目标连接
	if _, err := tunStream.remoteConn.Write(payload); err != nil {
		projlogger.Debug("转发TCP数据包失败: %v", err)
		// 如果写入失败，流可能已关闭，清理
		h.streamsMu.Lock()
		delete(h.streams, streamID)
		h.streamsMu.Unlock()
	}
}

// forwardRemoteToQuic 从远程连接读取数据并转发到QUIC流
func (h *TUNHandler) forwardRemoteToQuic(stream *TUNStream) {
	defer func() {
		close(stream.readDone)
		stream.Close()
	}()

	buffer := make([]byte, 32*1024) // 32KB缓冲区
	for {
		select {
		case <-h.ctx.Done():
			return
		default:
			n, err := stream.remoteConn.Read(buffer)
			if n > 0 {
				// 构建TUIC PACKET响应: [版本(1字节)][命令(1字节)][长度(2字节)][数据]
				packet := make([]byte, 4+n)
				packet[0] = 5 // 版本
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

// forwardQuicToRemote 从QUIC流读取数据并转发到远程连接
func (h *TUNHandler) forwardQuicToRemote(stream *TUNStream) {
	defer func() {
		close(stream.writeDone)
		stream.Close()
	}()

	buffer := make([]byte, 32*1024) // 32KB缓冲区
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

			command := header[1]
			dataLen := binary.BigEndian.Uint16(header[2:4])

			if command == 1 { // PACKET命令
				// 读取数据包
				if dataLen > uint16(len(buffer)) {
					projlogger.Debug("数据包太大: %d", dataLen)
					return
				}

				data := buffer[:dataLen]
				if _, err := io.ReadFull(stream.stream, data); err != nil {
					projlogger.Debug("读取数据包失败: %v", err)
					return
				}

				// 转发到远程连接
				if _, err := stream.remoteConn.Write(data); err != nil {
					projlogger.Debug("写入远程连接失败: %v", err)
					return
				}
			}
		}
	}
}

// Close 关闭TUN流
func (s *TUNStream) Close() {
	s.closeOnce.Do(func() {
		if s.remoteConn != nil {
			s.remoteConn.Close()
		}
		if s.stream != nil {
			s.stream.Close()
		}
	})
}
