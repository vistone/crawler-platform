package main

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"

	projlogger "crawler-platform/logger"
)

// forwardTCPFromRemote 从远程TCP连接读取数据并封装回IP包
func (h *IPTunnelHandler) forwardTCPFromRemote(tunnel *IPTunnel, tcpConn *TCPConnection) {
	defer func() {
		tcpConn.mu.Lock()
		if tcpConn.conn != nil {
			tcpConn.conn.Close()
			tcpConn.conn = nil
		}
		tcpConn.mu.Unlock()
	}()

	buffer := make([]byte, 32*1024) // 32KB缓冲区
	for {
		select {
		case <-h.ctx.Done():
			return
		default:
			tcpConn.mu.Lock()
			if tcpConn.conn == nil {
				tcpConn.mu.Unlock()
				return
			}
			conn := tcpConn.conn
			tcpConn.mu.Unlock()

			n, err := conn.Read(buffer)
			if n > 0 {
				// 构建TCP响应包
				tcpResp := h.buildTCPPacket(
					tcpConn.dstIP, tcpConn.dstPort,
					tcpConn.srcIP, tcpConn.srcPort,
					buffer[:n],
					false, false, true, // ACK标志
				)

				// 封装到IPv4包
				ipPacket := h.buildIPv4Packet(tcpConn.dstIP, tcpConn.srcIP, 6, tcpResp) // 6 = TCP

				// 发送回客户端
				if err := h.sendIPPacket(tunnel, ipPacket); err != nil {
					projlogger.Debug("发送TCP响应IP包失败: %v", err)
					return
				}
			}
			if err != nil {
				if err != io.EOF {
					projlogger.Debug("读取远程TCP连接失败: %v", err)
				}
				return
			}
		}
	}
}

// processOutgoingResponses 处理外发的响应（用于处理其他异步响应）
func (h *IPTunnelHandler) processOutgoingResponses(tunnel *IPTunnel) {
	defer func() {
		close(tunnel.writeDone)
	}()

	// 这个函数主要用于等待其他goroutine完成
	// 实际的数据发送在各自的处理函数中完成
	select {
	case <-h.ctx.Done():
		return
	}
}

// sendIPPacket 发送IP数据包到客户端
func (h *IPTunnelHandler) sendIPPacket(tunnel *IPTunnel, ipPacket []byte) error {
	// 构建TUIC PACKET命令: [版本(1字节)][命令(1字节)][长度(2字节)][IP数据包]
	packet := make([]byte, 4+len(ipPacket))
	packet[0] = 5 // 版本
	packet[1] = 1 // PACKET命令
	binary.BigEndian.PutUint16(packet[2:4], uint16(len(ipPacket)))
	copy(packet[4:], ipPacket)

	if _, err := tunnel.stream.Write(packet); err != nil {
		return fmt.Errorf("写入QUIC流失败: %w", err)
	}

	return nil
}

// buildIPv4Packet 构建IPv4数据包
func (h *IPTunnelHandler) buildIPv4Packet(srcIP, dstIP net.IP, protocol byte, payload []byte) []byte {
	// IPv4包头最小20字节
	headerLen := 20
	totalLen := headerLen + len(payload)

	packet := make([]byte, totalLen)

	// IPv4包头
	packet[0] = 0x45                                          // 版本(4) + IHL(5)
	packet[1] = 0x00                                          // TOS
	binary.BigEndian.PutUint16(packet[2:4], uint16(totalLen)) // 总长度
	packet[4] = 0x00                                          // 标识
	packet[5] = 0x00
	packet[6] = 0x40 // 标志 + 片偏移
	packet[7] = 0x00
	packet[8] = 64       // TTL
	packet[9] = protocol // 协议
	packet[10] = 0       // 校验和（简化，不计算）
	packet[11] = 0

	// 源IP地址
	if len(srcIP) == 4 {
		copy(packet[12:16], srcIP)
	} else {
		// IPv6映射到IPv4，使用最后4字节
		copy(packet[12:16], srcIP[12:16])
	}

	// 目标IP地址
	if len(dstIP) == 4 {
		copy(packet[16:20], dstIP)
	} else {
		// IPv6映射到IPv4，使用最后4字节
		copy(packet[16:20], dstIP[12:16])
	}

	// 负载
	copy(packet[20:], payload)

	return packet
}

// buildTCPPacket 构建TCP数据包
func (h *IPTunnelHandler) buildTCPPacket(srcIP net.IP, srcPort uint16, dstIP net.IP, dstPort uint16, payload []byte, syn, fin, ack bool) []byte {
	// TCP包头最小20字节
	headerLen := 20
	totalLen := headerLen + len(payload)

	packet := make([]byte, totalLen)

	// TCP包头
	binary.BigEndian.PutUint16(packet[0:2], srcPort) // 源端口
	binary.BigEndian.PutUint16(packet[2:4], dstPort) // 目标端口
	binary.BigEndian.PutUint32(packet[4:8], 0)       // 序列号（简化）
	binary.BigEndian.PutUint32(packet[8:12], 0)      // 确认号（简化）
	packet[12] = byte((headerLen / 4) << 4)          // 数据偏移
	packet[13] = 0                                   // 标志位
	if syn {
		packet[13] |= 0x02 // SYN
	}
	if fin {
		packet[13] |= 0x01 // FIN
	}
	if ack {
		packet[13] |= 0x10 // ACK
	}
	binary.BigEndian.PutUint16(packet[14:16], 65535) // 窗口大小
	packet[16] = 0                                   // 校验和（简化，不计算）
	packet[17] = 0
	packet[18] = 0 // 紧急指针
	packet[19] = 0

	// 负载
	if len(payload) > 0 {
		copy(packet[20:], payload)
	}

	return packet
}

// sendTCPRST 发送TCP RST响应
func (h *IPTunnelHandler) sendTCPRST(tunnel *IPTunnel, srcIP net.IP, srcPort uint16, dstIP net.IP, dstPort uint16) error {
	// 构建TCP RST包
	tcpRST := h.buildTCPPacket(
		dstIP, dstPort, // 源IP/端口变目标
		srcIP, srcPort, // 目标IP/端口变源
		nil,   // 无负载
		false, // 不是SYN
		false, // 不是FIN
		true,  // ACK标志
	)
	tcpRST[13] |= 0x04 // RST标志

	// 封装到IPv4包
	ipPacket := h.buildIPv4Packet(dstIP, srcIP, 6, tcpRST) // 6 = TCP

	// 发送回客户端
	return h.sendIPPacket(tunnel, ipPacket)
}
