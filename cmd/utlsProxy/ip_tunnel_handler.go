package main

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	projlogger "crawler-platform/logger"

	"github.com/quic-go/quic-go"
)

// IPTunnelHandler IP层隧道处理器
// 真正的TUN层实现：处理IP数据包，而不是TCP连接
type IPTunnelHandler struct {
	proxy     *UTLSProxy
	conn      *quic.Conn
	tunnels   map[quic.StreamID]*IPTunnel
	tunnelsMu sync.RWMutex
	ctx       context.Context
	cancel    context.CancelFunc
}

// IPTunnel IP隧道，对应一个网络连接
type IPTunnel struct {
	stream      *quic.Stream
	localIP     net.IP // 分配的虚拟IP
	remoteIP    net.IP // 客户端虚拟IP
	tcpConns    map[string]*TCPConnection // TCP连接映射: "srcIP:srcPort->dstIP:dstPort" -> connection
	tcpConnsMu  sync.RWMutex
	readDone    chan struct{}
	writeDone   chan struct{}
	closeOnce   sync.Once
}

// TCPConnection TCP连接状态
type TCPConnection struct {
	conn        net.Conn
	srcIP       net.IP
	srcPort     uint16
	dstIP       net.IP
	dstPort     uint16
	established bool
	mu          sync.Mutex
}

// UDPForwarder UDP转发器
type UDPForwarder struct {
	conn *net.UDPConn
}

// NewIPTunnelHandler 创建新的IP隧道处理器
func NewIPTunnelHandler(proxy *UTLSProxy, conn *quic.Conn) *IPTunnelHandler {
	ctx, cancel := context.WithCancel(context.Background())
	return &IPTunnelHandler{
		proxy:   proxy,
		conn:    conn,
		tunnels: make(map[quic.StreamID]*IPTunnel),
		ctx:     ctx,
		cancel:  cancel,
	}
}

// Start 启动IP隧道处理器
func (h *IPTunnelHandler) Start() {
	// 启动接受流的goroutine
	go h.acceptStreams()
}

// Stop 停止IP隧道处理器
func (h *IPTunnelHandler) Stop() {
	h.cancel()

	// 关闭所有隧道
	h.tunnelsMu.Lock()
	for _, tunnel := range h.tunnels {
		tunnel.Close()
	}
	h.tunnelsMu.Unlock()
}

// acceptStreams 接受QUIC流
func (h *IPTunnelHandler) acceptStreams() {
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

			// 处理每个流（作为IP数据包流）
			go h.handleIPPacketStream(stream)
		}
	}
}

// handleIPPacketStream 处理IP数据包流
func (h *IPTunnelHandler) handleIPPacketStream(stream *quic.Stream) {
	defer stream.Close()

	streamID := stream.StreamID()

	// 创建IP隧道
	tunnel := &IPTunnel{
		stream:   stream,
		tcpConns: make(map[string]*TCPConnection),
		readDone: make(chan struct{}),
		writeDone: make(chan struct{}),
	}

	// 注册隧道
	h.tunnelsMu.Lock()
	h.tunnels[streamID] = tunnel
	h.tunnelsMu.Unlock()

	// 启动IP数据包处理（双向）
	go h.processIncomingIPPackets(tunnel)
	go h.processOutgoingResponses(tunnel)

	// 等待完成
	<-tunnel.readDone
	<-tunnel.writeDone

	// 清理隧道
	h.tunnelsMu.Lock()
	delete(h.tunnels, streamID)
	h.tunnelsMu.Unlock()
}

// processIncomingIPPackets 处理从客户端收到的IP数据包
func (h *IPTunnelHandler) processIncomingIPPackets(tunnel *IPTunnel) {
	defer func() {
		close(tunnel.readDone)
		tunnel.Close()
	}()

	buffer := make([]byte, 64*1024) // 64KB缓冲区，足够存储最大IP包

	for {
		select {
		case <-h.ctx.Done():
			return
		default:
			// 读取TUIC协议头
			header := make([]byte, 4)
			if _, err := io.ReadFull(tunnel.stream, header); err != nil {
				if err != io.EOF {
					projlogger.Debug("读取TUIC协议头失败: %v", err)
				}
				return
			}

			version := header[0]
			command := header[1]
			dataLen := binary.BigEndian.Uint16(header[2:4])

			if version != 5 {
				projlogger.Error("不支持的TUIC版本: %d", version)
				return
			}

			if command == 1 { // PACKET命令，包含IP数据包
				// 读取IP数据包
				if dataLen > uint16(len(buffer)) {
					projlogger.Debug("IP数据包太大: %d", dataLen)
					return
				}

				ipPacket := buffer[:dataLen]
				if _, err := io.ReadFull(tunnel.stream, ipPacket); err != nil {
					projlogger.Debug("读取IP数据包失败: %v", err)
					return
				}

				// 处理IP数据包
				if err := h.handleIPPacket(tunnel, ipPacket); err != nil {
					projlogger.Debug("处理IP数据包失败: %v", err)
				}
			}
		}
	}
}

// handleIPPacket 处理单个IP数据包
func (h *IPTunnelHandler) handleIPPacket(tunnel *IPTunnel, packet []byte) error {
	if len(packet) < 20 {
		return fmt.Errorf("IP数据包太短")
	}

	// 解析IP包头
	version := (packet[0] >> 4) & 0x0F
	if version != 4 && version != 6 {
		return fmt.Errorf("不支持的IP版本: %d", version)
	}

	if version == 4 {
		return h.handleIPv4Packet(tunnel, packet)
	} else {
		return h.handleIPv6Packet(tunnel, packet)
	}
}

// handleIPv4Packet 处理IPv4数据包
func (h *IPTunnelHandler) handleIPv4Packet(tunnel *IPTunnel, packet []byte) error {
	if len(packet) < 20 {
		return fmt.Errorf("IPv4数据包太短")
	}

	// 解析IPv4包头
	ihl := int(packet[0] & 0x0F)
	if ihl < 5 {
		return fmt.Errorf("无效的IPv4包头长度: %d", ihl)
	}

	headerLen := ihl * 4
	if len(packet) < headerLen {
		return fmt.Errorf("IPv4数据包不完整")
	}

	protocol := packet[9] // 协议字段
	srcIP := net.IP(packet[12:16])
	dstIP := net.IP(packet[16:20])

	// 解析总长度
	totalLen := binary.BigEndian.Uint16(packet[2:4])
	if len(packet) < int(totalLen) {
		return fmt.Errorf("IPv4数据包长度不匹配")
	}

	// 提取负载
	payload := packet[headerLen:totalLen]

	projlogger.Debug("IPv4数据包: %s -> %s, 协议: %d, 负载长度: %d",
		srcIP, dstIP, protocol, len(payload))

	// 根据协议类型处理
	switch protocol {
	case 6: // TCP
		return h.handleTCPPacket(tunnel, srcIP, dstIP, payload)
	case 17: // UDP
		return h.handleUDPPacket(tunnel, srcIP, dstIP, payload)
	default:
		projlogger.Debug("不支持的协议类型: %d", protocol)
		return nil
	}
}

// handleIPv6Packet 处理IPv6数据包
func (h *IPTunnelHandler) handleIPv6Packet(tunnel *IPTunnel, packet []byte) error {
	if len(packet) < 40 {
		return fmt.Errorf("IPv6数据包太短")
	}

	// 解析IPv6包头
	srcIP := net.IP(packet[8:24])
	dstIP := net.IP(packet[24:40])
	nextHeader := packet[6] // 下一个头类型

	projlogger.Debug("IPv6数据包: %s -> %s, 下一个头: %d",
		srcIP, dstIP, nextHeader)

	// 简化处理，只处理TCP和UDP
	if nextHeader == 6 { // TCP
		return h.handleTCPPacket(tunnel, srcIP, dstIP, packet[40:])
	} else if nextHeader == 17 { // UDP
		return h.handleUDPPacket(tunnel, srcIP, dstIP, packet[40:])
	}

	return nil
}

// handleTCPPacket 处理TCP数据包，建立或使用TCP连接转发
func (h *IPTunnelHandler) handleTCPPacket(tunnel *IPTunnel, srcIP, dstIP net.IP, tcpData []byte) error {
	if len(tcpData) < 20 {
		return fmt.Errorf("TCP数据包太短")
	}

	// 解析TCP包头
	srcPort := binary.BigEndian.Uint16(tcpData[0:2])
	dstPort := binary.BigEndian.Uint16(tcpData[2:4])
	flags := tcpData[13] // TCP标志位

	dataOffset := (tcpData[12] >> 4) & 0x0F
	tcpHeaderLen := int(dataOffset) * 4
	if len(tcpData) < tcpHeaderLen {
		return fmt.Errorf("TCP包头不完整")
	}

	// 提取TCP负载
	payload := tcpData[tcpHeaderLen:]

	// 构建连接键
	connKey := fmt.Sprintf("%s:%d->%s:%d", srcIP, srcPort, dstIP, dstPort)

	// 检查TCP标志位
	syn := (flags & 0x02) != 0
	ack := (flags & 0x10) != 0
	fin := (flags & 0x01) != 0
	rst := (flags & 0x04) != 0

	tunnel.tcpConnsMu.Lock()
	tcpConn, exists := tunnel.tcpConns[connKey]
	tunnel.tcpConnsMu.Unlock()

	// 处理SYN（建立连接）
	if syn && !exists {
		targetAddr := fmt.Sprintf("%s:%d", dstIP, dstPort)
		projlogger.Debug("建立TCP连接: %s", targetAddr)

		conn, err := net.DialTimeout("tcp", targetAddr, h.proxy.config.PoolConfig.ConnTimeout)
		if err != nil {
			projlogger.Debug("建立TCP连接失败 %s: %v", targetAddr, err)
			// 发送RST响应
			return h.sendTCPRST(tunnel, srcIP, srcPort, dstIP, dstPort)
		}

		tcpConn = &TCPConnection{
			conn:        conn,
			srcIP:       srcIP,
			srcPort:     srcPort,
			dstIP:       dstIP,
			dstPort:     dstPort,
			established: false,
		}

		tunnel.tcpConnsMu.Lock()
		tunnel.tcpConns[connKey] = tcpConn
		tunnel.tcpConnsMu.Unlock()

		// 启动从目标服务器读取数据的goroutine
		go h.forwardTCPFromRemote(tunnel, tcpConn)

		// 如果SYN+ACK，连接已建立
		if ack {
			tcpConn.mu.Lock()
			tcpConn.established = true
			tcpConn.mu.Unlock()
		}
	}

	// 处理FIN或RST（关闭连接）
	if (fin || rst) && exists {
		projlogger.Debug("关闭TCP连接: %s", connKey)
		tcpConn.mu.Lock()
		if tcpConn.conn != nil {
			tcpConn.conn.Close()
			tcpConn.conn = nil
		}
		tcpConn.mu.Unlock()

		tunnel.tcpConnsMu.Lock()
		delete(tunnel.tcpConns, connKey)
		tunnel.tcpConnsMu.Unlock()

		return nil
	}

	// 转发数据
	if exists && len(payload) > 0 {
		tcpConn.mu.Lock()
		if tcpConn.conn != nil && tcpConn.established {
			if _, err := tcpConn.conn.Write(payload); err != nil {
				projlogger.Debug("转发TCP数据失败: %v", err)
				tcpConn.mu.Unlock()
				// 连接可能已断开，清理
				tunnel.tcpConnsMu.Lock()
				delete(tunnel.tcpConns, connKey)
				tunnel.tcpConnsMu.Unlock()
				return err
			}
		}
		tcpConn.mu.Unlock()
	}

	return nil
}

// handleUDPPacket 处理UDP数据包
func (h *IPTunnelHandler) handleUDPPacket(tunnel *IPTunnel, srcIP, dstIP net.IP, udpData []byte) error {
	if len(udpData) < 8 {
		return fmt.Errorf("UDP数据包太短")
	}

	// 解析UDP包头
	srcPort := binary.BigEndian.Uint16(udpData[0:2])
	dstPort := binary.BigEndian.Uint16(udpData[2:4])
	length := binary.BigEndian.Uint16(udpData[4:6])

	if len(udpData) < int(length) {
		return fmt.Errorf("UDP数据包长度不匹配")
	}

	// 提取UDP负载
	payload := udpData[8:length]

	// 构建目标地址
	targetAddr := fmt.Sprintf("%s:%d", dstIP, dstPort)

	projlogger.Debug("UDP数据包: %s:%d -> %s, 负载长度: %d",
		srcIP, srcPort, targetAddr, len(payload))

	// UDP是无连接的，直接转发
	udpAddr, err := net.ResolveUDPAddr("udp", targetAddr)
	if err != nil {
		return fmt.Errorf("解析UDP地址失败: %w", err)
	}

	conn, err := net.DialUDP("udp", nil, udpAddr)
	if err != nil {
		return fmt.Errorf("建立UDP连接失败: %w", err)
	}
	defer conn.Close()

	// 发送数据
	if _, err := conn.Write(payload); err != nil {
		return fmt.Errorf("发送UDP数据失败: %w", err)
	}

	// 读取响应（异步，避免阻塞）
	go h.readUDPResponse(tunnel, conn, srcIP, srcPort, dstIP, dstPort)

	return nil
}

// readUDPResponse 读取UDP响应并封装回IP包
func (h *IPTunnelHandler) readUDPResponse(tunnel *IPTunnel, conn *net.UDPConn, srcIP net.IP, srcPort uint16, dstIP net.IP, dstPort uint16) {
	buffer := make([]byte, 64*1024)
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	
	n, err := conn.Read(buffer)
	if err != nil {
		projlogger.Debug("读取UDP响应失败: %v", err)
		return
	}

	// 构建UDP响应包
	udpResp := make([]byte, 8+n)
	binary.BigEndian.PutUint16(udpResp[0:2], dstPort) // 源端口变目标端口
	binary.BigEndian.PutUint16(udpResp[2:4], srcPort) // 目标端口变源端口
	binary.BigEndian.PutUint16(udpResp[4:6], uint16(8+n))
	udpResp[6] = 0 // 校验和（简化，不计算）
	udpResp[7] = 0
	copy(udpResp[8:], buffer[:n])

	// 封装到IPv4包
	ipPacket := h.buildIPv4Packet(dstIP, srcIP, 17, udpResp) // 17 = UDP

	// 发送回客户端
	if err := h.sendIPPacket(tunnel, ipPacket); err != nil {
		projlogger.Debug("发送UDP响应IP包失败: %v", err)
	}
}

// Close 关闭IP隧道
func (t *IPTunnel) Close() {
	t.closeOnce.Do(func() {
		if t.stream != nil {
			t.stream.Close()
		}
	})
}
