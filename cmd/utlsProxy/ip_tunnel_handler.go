package main

import (
	"context"
	"crypto/tls"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"sync"
	"time"

	projlogger "crawler-platform/logger"

	"github.com/quic-go/quic-go"
)

// #region agent log helper
func writeDebugLog(id, hypothesisId, location, message string, data map[string]interface{}) {
	logPath := "/home/stone/crawler-platform/.cursor/debug.log"
	logEntry := map[string]interface{}{
		"id":           id,
		"timestamp":    time.Now().UnixMilli(),
		"location":     location,
		"message":      message,
		"data":         data,
		"sessionId":    "debug-session",
		"runId":        "run1",
		"hypothesisId": hypothesisId,
	}
	jsonData, _ := json.Marshal(logEntry)
	f, _ := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if f != nil {
		fmt.Fprintf(f, "%s\n", jsonData)
		f.Close()
	}
}

// #endregion agent log

// IPTunnelHandler TUIC协议处理器
// 处理TUIC协议的CONNECT和PACKET命令
// - CONNECT命令：建立TCP连接（TCP代理模式）
// - PACKET命令：转发IP数据包（IP隧道模式）
// TUIC是应用层（L7）协议，构建在QUIC传输层（L4）之上
// TUN设备是网络层（L3）设备，用于捕获/注入IP数据包
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
	stream     *quic.Stream
	localIP    net.IP                    // 分配的虚拟IP
	remoteIP   net.IP                    // 客户端虚拟IP
	tcpConns   map[string]*TCPConnection // TCP连接映射: "srcIP:srcPort->dstIP:dstPort" -> connection
	tcpConnsMu sync.RWMutex
	readDone   chan struct{}
	writeDone  chan struct{}
	closeOnce  sync.Once
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
	projlogger.Info("IPTunnelHandler: 开始接受QUIC流")
	for {
		select {
		case <-h.ctx.Done():
			projlogger.Info("IPTunnelHandler: 上下文已取消，停止接受流")
			return
		default:
			stream, err := h.conn.AcceptStream(h.ctx)
			if err != nil {
				if h.ctx.Err() != nil {
					projlogger.Info("IPTunnelHandler: 上下文已取消，停止接受流")
					return
				}
				projlogger.Error("IPTunnelHandler: 接受QUIC流失败: %v", err)
				continue
			}

			streamID := stream.StreamID()
			projlogger.Info("IPTunnelHandler: 接受新QUIC流，StreamID: %d", streamID)
			// #region agent log
			writeDebugLog("log_proxy_stream_accepted", "H1", "ip_tunnel_handler.go:acceptStreams", "Accepted new QUIC stream", map[string]interface{}{"streamID": streamID})
			// #endregion agent log

			// 处理每个流（自动识别CONNECT或PACKET模式）
			go h.handleIPPacketStream(stream)
		}
	}
}

// handleIPPacketStream 处理数据流（兼容CONNECT和PACKET两种模式）
func (h *IPTunnelHandler) handleIPPacketStream(stream *quic.Stream) {
	defer stream.Close()

	streamID := stream.StreamID()
	projlogger.Info("IPTunnelHandler: 开始处理流，StreamID: %d", streamID)
	// #region agent log
	writeDebugLog("log_proxy_entry", "H1", "ip_tunnel_handler.go:handleIPPacketStreamEntry", "Stream processing started", map[string]interface{}{"streamID": streamID})
	// #endregion agent log

	// 读取第一个TUIC协议头来识别模式
	header := make([]byte, 4)
	if _, err := io.ReadFull(stream, header); err != nil {
		projlogger.Error("IPTunnelHandler: 读取TUIC协议头失败 (StreamID: %d): %v", streamID, err)
		// #region agent log
		writeDebugLog("log_proxy_header_read_fail", "H1", "ip_tunnel_handler.go:handleIPPacketStreamHeaderReadFail", "Failed to read TUIC header", map[string]interface{}{"streamID": streamID, "error": err.Error()})
		// #endregion agent log
		return
	}

	version := header[0]
	command := header[1]
	dataLen := binary.BigEndian.Uint16(header[2:4])
	projlogger.Info("IPTunnelHandler: 收到TUIC协议头 (StreamID: %d, Version: %d, Command: %d, DataLen: %d)", streamID, version, command, dataLen)
	// #region agent log
	writeDebugLog("log_proxy_header", "H1", "ip_tunnel_handler.go:handleIPPacketStreamHeader", "Received initial TUIC header", map[string]interface{}{"streamID": streamID, "version": version, "command": command, "dataLen": dataLen})
	// #endregion agent log

	if version != 5 {
		projlogger.Error("IPTunnelHandler: 不支持的TUIC版本: %d (StreamID: %d)", version, streamID)
		return
	}

	// 读取payload
	payload := make([]byte, dataLen)
	if _, err := io.ReadFull(stream, payload); err != nil {
		projlogger.Error("IPTunnelHandler: 读取payload失败 (StreamID: %d, DataLen: %d): %v", streamID, dataLen, err)
		// #region agent log
		writeDebugLog("log_proxy_payload_read_fail", "H1", "ip_tunnel_handler.go:handleIPPacketStreamPayloadReadFail", "Failed to read payload", map[string]interface{}{"streamID": streamID, "dataLen": dataLen, "error": err.Error()})
		// #endregion agent log
		return
	}
	projlogger.Info("IPTunnelHandler: 成功读取payload (StreamID: %d, PayloadLen: %d)", streamID, len(payload))
	// #region agent log
	writeDebugLog("log_proxy_payload", "H1", "ip_tunnel_handler.go:handleIPPacketStreamPayload", "Received initial TUIC payload", map[string]interface{}{"streamID": streamID, "payloadLen": len(payload)})
	// #endregion agent log

	// 根据命令类型选择处理方式
	if command == 0 {
		// CONNECT命令：TCP代理模式（向后兼容utlsclient）
		projlogger.Info("IPTunnelHandler: 检测到CONNECT命令 (StreamID: %d)，进入TCP代理模式", streamID)
		// #region agent log
		writeDebugLog("log_proxy_mode_connect", "H1", "ip_tunnel_handler.go:handleIPPacketStreamMode", "Detected CONNECT command, entering TCP proxy mode", map[string]interface{}{"streamID": streamID})
		// #endregion agent log
		h.handleConnectCommand(streamID, stream, payload)
		return
	} else if command == 1 {
		// PACKET命令：真正的TUN模式（IP数据包）
		projlogger.Info("IPTunnelHandler: 检测到PACKET命令 (StreamID: %d)，进入TUN模式", streamID)
		// #region agent log
		writeDebugLog("log_proxy_mode_packet", "H1", "ip_tunnel_handler.go:handleIPPacketStreamMode", "Detected PACKET command, entering TUN mode", map[string]interface{}{"streamID": streamID})
		// #endregion agent log
		h.handleFirstPacketForTunnel(streamID, stream, payload)
		return
	} else {
		projlogger.Error("IPTunnelHandler: 不支持的TUIC命令: %d (StreamID: %d)", command, streamID)
		return
	}
}

// handleFirstPacketForTunnel 处理第一个PACKET并创建IP隧道
func (h *IPTunnelHandler) handleFirstPacketForTunnel(streamID quic.StreamID, stream *quic.Stream, firstPacket []byte) {
	// 创建IP隧道
	tunnel := &IPTunnel{
		stream:    stream,
		tcpConns:  make(map[string]*TCPConnection),
		readDone:  make(chan struct{}),
		writeDone: make(chan struct{}),
	}

	// 注册隧道
	h.tunnelsMu.Lock()
	h.tunnels[streamID] = tunnel
	h.tunnelsMu.Unlock()

	// 处理第一个数据包
	// #region agent log
	writeDebugLog("log_proxy_packet_mode_init", "H1", "ip_tunnel_handler.go:handleFirstPacketForTunnelEntry", "Initialized TUN mode for stream", map[string]interface{}{"streamID": streamID})
	// #endregion agent log
	if err := h.handleIPPacket(tunnel, firstPacket); err != nil {
		projlogger.Debug("处理第一个IP数据包失败: %v", err)
	}
	// #region agent log
	writeDebugLog("log_proxy_packet_mode_first_packet_handled", "H1", "ip_tunnel_handler.go:handleFirstPacketForTunnelPacketHandled", "Handled first IP packet in TUN mode", map[string]interface{}{"streamID": streamID})
	// #endregion agent log

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

// handleConnectCommand 处理CONNECT命令（TCP代理模式，用于向后兼容）
func (h *IPTunnelHandler) handleConnectCommand(streamID quic.StreamID, stream *quic.Stream, payload []byte) {
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

	projlogger.Info("IPTunnelHandler: CONNECT命令 (TCP代理模式, StreamID: %d): 目标地址 %s", streamID, target)
	// #region agent log
	writeDebugLog("log_proxy_connect_target", "H1", "ip_tunnel_handler.go:handleConnectCommandTarget", "Connecting to target", map[string]interface{}{"streamID": streamID, "target": target})
	// #endregion agent log

	// 建立TCP连接
	projlogger.Info("IPTunnelHandler: 正在连接到目标服务器 (StreamID: %d, Target: %s)", streamID, target)
	tcpConn, err := net.DialTimeout("tcp", target, h.proxy.config.PoolConfig.ConnTimeout)
	if err != nil {
		projlogger.Error("IPTunnelHandler: 连接到目标服务器失败 (StreamID: %d, Target: %s): %v", streamID, target, err)
		// #region agent log
		writeDebugLog("log_proxy_connect_fail", "H1", "ip_tunnel_handler.go:handleConnectCommandDialFail", "Failed to dial target", map[string]interface{}{"streamID": streamID, "target": target, "error": err.Error()})
		// #endregion agent log
		errorResp := make([]byte, 4)
		errorResp[0] = 5
		errorResp[1] = 2 // 错误响应
		binary.BigEndian.PutUint16(errorResp[2:4], 0)
		stream.Write(errorResp)
		return
	}

	// 检测是否为HTTPS（端口443），如果是则建立TLS连接
	var remoteConn net.Conn = tcpConn
	if port == 443 {
		projlogger.Info("IPTunnelHandler: 检测到HTTPS目标，建立TLS连接 (StreamID: %d, Target: %s)", streamID, target)
		tlsConfig := &tls.Config{
			ServerName:         targetAddr,
			InsecureSkipVerify: false,
		}
		tlsConn := tls.Client(tcpConn, tlsConfig)
		if err := tlsConn.Handshake(); err != nil {
			projlogger.Error("IPTunnelHandler: TLS握手失败 (StreamID: %d, Target: %s): %v", streamID, target, err)
			tcpConn.Close()
			errorResp := make([]byte, 4)
			errorResp[0] = 5
			errorResp[1] = 2 // 错误响应
			binary.BigEndian.PutUint16(errorResp[2:4], 0)
			stream.Write(errorResp)
			return
		}
		remoteConn = tlsConn
		projlogger.Info("IPTunnelHandler: TLS握手成功 (StreamID: %d, Target: %s)", streamID, target)
	}
	projlogger.Info("IPTunnelHandler: 成功连接到目标服务器 (StreamID: %d, Target: %s)", streamID, target)
	// #region agent log
	writeDebugLog("log_proxy_connect_success", "H1", "ip_tunnel_handler.go:handleConnectCommandDialSuccess", "Successfully dialed target", map[string]interface{}{"streamID": streamID, "target": target})
	// #endregion agent log

	// 发送成功响应
	successResp := make([]byte, 4)
	successResp[0] = 5
	successResp[1] = 0 // CONNECT成功
	binary.BigEndian.PutUint16(successResp[2:4], 0)
	if _, err := stream.Write(successResp); err != nil {
		projlogger.Error("IPTunnelHandler: 发送CONNECT成功响应失败 (StreamID: %d): %v", streamID, err)
		remoteConn.Close()
		return
	}
	projlogger.Info("IPTunnelHandler: 已发送CONNECT成功响应 (StreamID: %d)", streamID)
	// #region agent log
	writeDebugLog("log_proxy_connect_resp_sent", "H1", "ip_tunnel_handler.go:handleConnectCommandSuccessResp", "Sent CONNECT success response", map[string]interface{}{"streamID": streamID})
	// #endregion agent log

	// 处理HTTP请求数据（在payload的剩余部分）
	httpData := payload[2+addrLen+2:]
	hasInitialData := len(httpData) > 0
	
	if hasInitialData {
		projlogger.Info("IPTunnelHandler: 写入初始HTTP数据到目标服务器 (StreamID: %d, DataLen: %d)", streamID, len(httpData))
		// #region agent log
		writeDebugLog("log_proxy_http_data_write", "H1", "ip_tunnel_handler.go:handleConnectCommandWriteHTTPData", "Writing initial HTTP data to target", map[string]interface{}{"streamID": streamID, "dataLen": len(httpData)})
		// #endregion agent log
		if _, err := remoteConn.Write(httpData); err != nil {
			projlogger.Error("IPTunnelHandler: 写入HTTP数据失败 (StreamID: %d): %v", streamID, err)
			remoteConn.Close()
			return
		}
		projlogger.Info("IPTunnelHandler: 成功写入初始HTTP数据 (StreamID: %d)", streamID)
	} else {
		projlogger.Info("IPTunnelHandler: payload中没有HTTP数据，等待后续数据 (StreamID: %d)", streamID)
	}

	// 启动双向数据转发
	readDone := make(chan struct{})
	writeDone := make(chan struct{})

	// 从远程读取并转发到QUIC流
	go func() {
		defer func() {
			close(readDone)
			remoteConn.Close()
			projlogger.Info("IPTunnelHandler: 远程到QUIC转发goroutine结束 (StreamID: %d)", streamID)
		}()

		projlogger.Info("IPTunnelHandler: 启动远程到QUIC转发goroutine (StreamID: %d)", streamID)
		buffer := make([]byte, 32*1024)
		
		for {
			select {
			case <-h.ctx.Done():
				projlogger.Info("IPTunnelHandler: 上下文已取消，停止远程到QUIC转发 (StreamID: %d)", streamID)
				return
			default:
				// 设置读取超时（每次读取前更新），避免无限等待
				// 对于TLS连接，SetReadDeadline会传播到底层连接
				remoteConn.SetReadDeadline(time.Now().Add(5 * time.Second))
				
				n, err := remoteConn.Read(buffer)
				if n > 0 {
					projlogger.Debug("IPTunnelHandler: 从远程连接读取数据 (StreamID: %d, BytesRead: %d)", streamID, n)
					// #region agent log
					writeDebugLog("log_proxy_forward_remote_read", "H1", "ip_tunnel_handler.go:forwardRemoteToQuicRead", "Read from remote TCP, preparing TUIC PACKET", map[string]interface{}{"streamID": stream.StreamID(), "bytesRead": n})
					// #endregion agent log
					// 构建TUIC PACKET响应
					packet := make([]byte, 4+n)
					packet[0] = 5
					packet[1] = 1 // PACKET命令
					binary.BigEndian.PutUint16(packet[2:4], uint16(n))
					copy(packet[4:], buffer[:n])

					if _, err := stream.Write(packet); err != nil {
						// #region agent log
						writeDebugLog("log_proxy_forward_remote_write_fail", "H1", "ip_tunnel_handler.go:forwardRemoteToQuicWriteFail", "Failed to write TUIC PACKET to QUIC stream", map[string]interface{}{"streamID": stream.StreamID(), "error": err.Error()})
						// #endregion agent log
						projlogger.Error("IPTunnelHandler: 写入QUIC流失败 (StreamID: %d): %v", streamID, err)
						return
					}
					projlogger.Debug("IPTunnelHandler: 成功写入PACKET到QUIC流 (StreamID: %d, PacketLen: %d)", streamID, len(packet))
					continue // 继续读取
				}
				
				if err != nil {
					if err == io.EOF {
						projlogger.Info("IPTunnelHandler: 远程连接已关闭 (StreamID: %d)，关闭QUIC流写入端", streamID)
						// 关闭QUIC流的写入端，告知客户端数据已传输完成
						stream.Close()
						return
					}
					// 检查是否是超时错误
					if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
						// 读取超时，可能响应已完整，关闭连接
						projlogger.Info("IPTunnelHandler: 读取超时，响应可能已完整 (StreamID: %d)，关闭QUIC流写入端", streamID)
						stream.Close()
						return
					}
					projlogger.Error("IPTunnelHandler: 读取远程连接失败 (StreamID: %d): %v", streamID, err)
					return
				}
			}
		}
	}()

	// 从QUIC流读取并转发到远程（仅在payload中没有初始数据时启动）
	// 如果payload中已经有HTTP数据，客户端不会再发送更多数据，所以不需要这个goroutine
	if !hasInitialData {
		go func() {
			defer func() {
				close(writeDone)
				remoteConn.Close()
				projlogger.Info("IPTunnelHandler: QUIC到远程转发goroutine结束 (StreamID: %d)", streamID)
			}()

			projlogger.Info("IPTunnelHandler: 启动QUIC到远程转发goroutine (StreamID: %d)", streamID)
			buffer := make([]byte, 32*1024)
			for {
				select {
				case <-h.ctx.Done():
					projlogger.Info("IPTunnelHandler: 上下文已取消，停止QUIC到远程转发 (StreamID: %d)", streamID)
					return
				default:
					// 读取TUIC协议头
					header := make([]byte, 4)
					if _, err := io.ReadFull(stream, header); err != nil {
						if err != io.EOF {
							// #region agent log
							writeDebugLog("log_proxy_forward_quic_read_header_fail", "H1", "ip_tunnel_handler.go:forwardQuicToRemoteReadHeaderFail", "Failed to read QUIC stream header", map[string]interface{}{"streamID": stream.StreamID(), "error": err.Error()})
							// #endregion agent log
							projlogger.Error("IPTunnelHandler: 读取QUIC流头失败 (StreamID: %d): %v", streamID, err)
						} else {
							projlogger.Info("IPTunnelHandler: QUIC流已关闭 (StreamID: %d)", streamID)
						}
						return
					}

					cmd := header[1]
					dataLen := binary.BigEndian.Uint16(header[2:4])
					projlogger.Info("IPTunnelHandler: 从QUIC流读取协议头 (StreamID: %d, Command: %d, DataLen: %d)", streamID, cmd, dataLen)
					// #region agent log
					writeDebugLog("log_proxy_forward_quic_header", "H1", "ip_tunnel_handler.go:forwardQuicToRemoteHeader", "Read QUIC stream header for forwarding", map[string]interface{}{"streamID": stream.StreamID(), "command": cmd, "dataLen": dataLen})
					// #endregion agent log

					if cmd == 1 { // PACKET命令
						if dataLen > uint16(len(buffer)) {
							projlogger.Error("IPTunnelHandler: 数据包太大 (StreamID: %d, DataLen: %d, BufferSize: %d)", streamID, dataLen, len(buffer))
							return
						}

						data := buffer[:dataLen]
						if _, err := io.ReadFull(stream, data); err != nil {
							// #region agent log
							writeDebugLog("log_proxy_forward_quic_read_data_fail", "H1", "ip_tunnel_handler.go:forwardQuicToRemoteReadDataFail", "Failed to read data from QUIC stream", map[string]interface{}{"streamID": stream.StreamID(), "error": err.Error()})
							// #endregion agent log
							projlogger.Error("IPTunnelHandler: 读取数据包失败 (StreamID: %d): %v", streamID, err)
							return
						}

						projlogger.Info("IPTunnelHandler: 从QUIC流读取数据包 (StreamID: %d, DataLen: %d)，准备写入远程连接", streamID, dataLen)
						if _, err := remoteConn.Write(data); err != nil {
							// #region agent log
							writeDebugLog("log_proxy_forward_quic_write_remote_fail", "H1", "ip_tunnel_handler.go:forwardQuicToRemoteWriteRemoteFail", "Failed to write data to remote connection", map[string]interface{}{"streamID": stream.StreamID(), "error": err.Error()})
							// #endregion agent log
							projlogger.Error("IPTunnelHandler: 写入远程连接失败 (StreamID: %d): %v", streamID, err)
							return
						}
						projlogger.Info("IPTunnelHandler: 成功写入数据到远程连接 (StreamID: %d, DataLen: %d)", streamID, dataLen)
					} else {
						projlogger.Warn("IPTunnelHandler: 收到非PACKET命令 (StreamID: %d, Command: %d)", streamID, cmd)
					}
				}
			}
		}()
	} else {
		// payload中已有HTTP数据，客户端不会再发送更多数据，直接关闭writeDone
		projlogger.Info("IPTunnelHandler: payload中已有HTTP数据，不需要从QUIC流读取更多数据 (StreamID: %d)", streamID)
		close(writeDone)
	}

	// 等待完成
	<-readDone
	<-writeDone
}

// processIncomingIPPackets 处理从客户端收到的IP数据包（仅用于PACKET模式）
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
			} else {
				projlogger.Debug("PACKET模式收到非PACKET命令: %d", command)
				return
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
