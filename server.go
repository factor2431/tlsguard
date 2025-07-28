package main

import (
	"bytes"
	"crypto/tls"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"net/netip"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"
	tun "github.com/sagernet/sing-tun"
)

type Server struct {
	sync.WaitGroup

	id                    uuid.UUID
	ln                    net.Listener
	device                *tun.NativeTun
	status                bool
	connections           *ConnectionPool
	deviceToRemoteChannel chan []byte
	remoteToDeviceChannel chan []byte
}

func NewServer(cfg *Config) (*Server, error) {
	ipv4 := make([]netip.Prefix, 0, len(cfg.IPv4))
	for _, i := range cfg.IPv4 {
		c, err := netip.ParsePrefix(i)
		if err != nil {
			return nil, fmt.Errorf("netip.ParsePrefix: %v", err)
		}
		ipv4 = append(ipv4, c)
	}
	ipv6 := make([]netip.Prefix, 0, len(cfg.IPv6))
	for _, i := range cfg.IPv6 {
		c, err := netip.ParsePrefix(i)
		if err != nil {
			return nil, fmt.Errorf("netip.ParsePrefix: %v", err)
		}
		ipv6 = append(ipv6, c)
	}

	tlsCrt, err := tls.LoadX509KeyPair(cfg.Certificate.Crt, cfg.Certificate.Key)
	if err != nil {
		return nil, fmt.Errorf("tls.LoadX509KeyPair: %v", err)
	}

	tlsSrv, err := tls.Listen("tcp", net.JoinHostPort(cfg.ListenAddr, strconv.Itoa(cfg.ListenPort)), &tls.Config{
		Certificates: []tls.Certificate{tlsCrt},
	})
	if err != nil {
		return nil, fmt.Errorf("tls.Listen: %v", err)
	}

	device, err := tun.New(tun.Options{
		Name:         cfg.Name,
		Inet4Address: ipv4,
		Inet6Address: ipv6,
		MTU:          uint32(cfg.MTU),
	})
	if err != nil {
		return nil, fmt.Errorf("tun.New: %v", err)
	}
	if err := device.Start(); err != nil {
		return nil, fmt.Errorf("device.Start: %v", err)
	}

	server := &Server{
		id:                    uuid.MustParse(cfg.ID),
		ln:                    tlsSrv,
		device:                device.(*tun.NativeTun),
		status:                true,
		connections:           NewConnectionPool(),
		deviceToRemoteChannel: make(chan []byte, cfg.DeviceToRemoteBufferSize),
		remoteToDeviceChannel: make(chan []byte, cfg.RemoteToDeviceBufferSize),
	}
	go server.acceptConn()
	go server.handleDevicePacket()
	for range cfg.Threads {
		go server.handleDeviceToRemoteChannel()
	}
	go server.handleRemoteToDeviceChannel()
	return server, nil
}

func (c *Server) Close() {
	c.status = false

	c.ln.Close()
	c.device.Close()
	close(c.deviceToRemoteChannel)
	close(c.remoteToDeviceChannel)

	c.connections.RemoveAndCloseAll()
	c.Wait()
	c.connections.RemoveAndCloseAll()

	for i := range c.deviceToRemoteChannel {
		if i == nil {
			break
		}
	}
	for i := range c.remoteToDeviceChannel {
		if i == nil {
			break
		}
	}
}

func (c *Server) acceptConn() {
	c.Add(1)
	defer c.Done()

	for c.status {
		conn, err := c.ln.Accept()
		if err != nil {
			break
		}

		go c.handleConn(conn)
	}
}

func (c *Server) handleConn(conn net.Conn) {
	c.Add(1)
	defer c.Done()

	var uuid uuid.UUID
	conn.SetReadDeadline(time.Now().Add(time.Second * 3))
	if _, err := io.ReadFull(conn, uuid[:]); err != nil {
		conn.Close()
		return
	}
	conn.SetReadDeadline(time.Time{})

	if !bytes.Equal(uuid[:], c.id[:]) {
		conn.Close()
		return
	}

	uuid = c.connections.Add(conn)
	defer c.connections.RemoveAndClose(uuid)

	for {
		data := make([]byte, 0xFFFF)
		if _, err := io.ReadFull(conn, data[:2]); err != nil {
			break
		}

		size := binary.BigEndian.Uint16(data[:2])
		if size <= 0 {
			break
		}

		if _, err := io.ReadFull(conn, data[:size]); err != nil {
			break
		}

		select {
		case c.remoteToDeviceChannel <- data[:size]:
			continue
		default:
			log.Printf("[server] remote to device channel full, kinda increase buffer size")
			continue
		}
	}
}

func (c *Server) handleDevicePacket() {
	c.Add(1)
	defer c.Done()

	for c.status {
		data := make([]byte, 0xFFFF)
		size, err := c.device.Read(data[2:])
		if err != nil {
			break
		}
		if size <= 0 {
			continue
		}
		binary.BigEndian.PutUint16(data[:2], uint16(size))

		select {
		case c.deviceToRemoteChannel <- data[:2+size]:
			continue
		default:
			log.Printf("[server] device to remote channel full, kinda increase buffer size")
			continue
		}
	}
}

func (c *Server) handleDeviceToRemoteChannel() {
	c.Add(1)
	defer c.Done()

	for c.status {
		data := <-c.deviceToRemoteChannel
		if data == nil {
			continue
		}

		conn := c.connections.Rand()
		if conn == nil {
			continue
		}

		if _, err := conn.Write(data); err != nil {
			c.connections.RemoveAndClose(conn.ID)
		}
	}
}

func (c *Server) handleRemoteToDeviceChannel() {
	c.Add(1)
	defer c.Done()

	for c.status {
		data := <-c.remoteToDeviceChannel
		if data == nil {
			break
		}

		if _, err := c.device.Write(data); err != nil {
			continue
		}
	}
}
