package main

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"net/netip"
	"os"
	"sync"
	"time"

	"github.com/google/uuid"
	tun "github.com/sagernet/sing-tun"
)

type Client struct {
	sync.WaitGroup

	id                    uuid.UUID
	device                *tun.NativeTun
	dialer                *tls.Dialer
	status                bool
	endpoint              string
	connections           *ConnectionPool
	connectionsCount      int
	deviceToRemoteChannel chan []byte
	remoteToDeviceChannel chan []byte
}

func NewClient(cfg *Config) (*Client, error) {
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

	client := &Client{
		id:     uuid.MustParse(cfg.ID),
		device: device.(*tun.NativeTun),
		dialer: &tls.Dialer{
			NetDialer: &net.Dialer{
				Timeout: time.Second * 10,
			},
			Config: &tls.Config{
				InsecureSkipVerify: cfg.Insecure,
			},
		},
		status:                true,
		endpoint:              cfg.Endpoint,
		connections:           NewConnectionPool(),
		connectionsCount:      cfg.ConnectionCount,
		deviceToRemoteChannel: make(chan []byte, cfg.DeviceToRemoteBufferSize),
		remoteToDeviceChannel: make(chan []byte, cfg.RemoteToDeviceBufferSize),
	}
	{
		list, _ := x509.SystemCertPool()
		if list == nil {
			list = x509.NewCertPool()
		}

		data, err := os.ReadFile(cfg.CABundle)
		if err == nil {
			list.AppendCertsFromPEM(data)
		}

		client.dialer.Config.RootCAs = list
	}
	{
		host, _, _ := net.SplitHostPort(cfg.Endpoint)
		if net.ParseIP(host) == nil {
			client.dialer.Config.ServerName = host
		}
	}
	go client.dialConn()
	go client.handleDevicePacket()
	for range cfg.Threads {
		go client.handleDeviceToRemoteChannel()
	}
	go client.handleRemoteToDeviceChannel()
	return client, nil
}

func (c *Client) Close() {
	c.status = false
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

func (c *Client) dialConn() {
	c.Add(1)
	defer c.Done()

	for c.status {
		size := c.connectionsCount - c.connections.Count()
		if size > 0 {
			for range size {
				go c.doDialConn()
			}
		}

		time.Sleep(time.Second * 1)
	}
}

func (c *Client) doDialConn() {
	c.Add(1)
	defer c.Done()

	conn, err := c.dialer.Dial("tcp", c.endpoint)
	if err != nil {
		return
	}

	if _, err := conn.Write(c.id[:]); err != nil {
		conn.Close()
		return
	}

	uuid := c.connections.Add(conn)
	go c.handleRemotePacket(uuid, conn)
}

func (c *Client) handleDevicePacket() {
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
			log.Printf("[client] device to remote channel full, kinda increase buffer size")
			continue
		}
	}
}

func (c *Client) handleRemotePacket(uuid uuid.UUID, conn net.Conn) {
	c.Add(1)
	defer c.Done()
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
			log.Printf("[client] remote to device channel full, kinda increase buffer size")
			continue
		}
	}
}

func (c *Client) handleDeviceToRemoteChannel() {
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

func (c *Client) handleRemoteToDeviceChannel() {
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
