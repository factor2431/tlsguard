package main

import (
	"math/rand/v2"
	"net"
	"sync"

	"github.com/google/uuid"
)

type ConnectionPool struct {
	sync.RWMutex

	list []*Connection
}

func NewConnectionPool() *ConnectionPool {
	return &ConnectionPool{}
}

func (c *ConnectionPool) Add(conn net.Conn) uuid.UUID {
	c.Lock()
	defer c.Unlock()

	connection := NewConnection(conn)
	c.list = append(c.list, connection)
	return connection.ID
}

func (c *ConnectionPool) Rand() *Connection {
	c.RLock()
	defer c.RUnlock()

	if len(c.list) <= 0 {
		return nil
	}

	return c.list[rand.IntN(len(c.list))]
}

func (c *ConnectionPool) Count() int {
	c.RLock()
	defer c.RUnlock()

	return len(c.list)
}

func (c *ConnectionPool) Remove(uuid uuid.UUID) *Connection {
	c.Lock()
	defer c.Unlock()

	var connection *Connection = nil
	list := make([]*Connection, 0, len(c.list))
	for _, conn := range c.list {
		if conn.ID == uuid {
			connection = conn
			continue
		}

		list = append(list, conn)
	}
	c.list = list

	return connection
}

func (c *ConnectionPool) RemoveAndClose(uuid uuid.UUID) {
	if conn := c.Remove(uuid); conn != nil {
		conn.Close()
	}
}

func (c *ConnectionPool) RemoveAndCloseAll() {
	c.Lock()
	defer c.Unlock()

	for _, conn := range c.list {
		conn.Close()
	}
	c.list = make([]*Connection, 0)
}
