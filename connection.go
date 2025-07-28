package main

import (
	"net"

	"github.com/google/uuid"
)

type Connection struct {
	net.Conn

	ID uuid.UUID
}

func NewConnection(conn net.Conn) *Connection {
	return &Connection{
		Conn: conn,

		ID: uuid.Must(uuid.NewRandom()),
	}
}
