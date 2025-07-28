package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/goccy/go-yaml"
)

func client(cfg *Config) {
	ctx, err := NewClient(cfg)
	if err != nil {
		log.Printf("[client] %v", err)
		return
	}
	defer ctx.Close()

	channel := make(chan os.Signal, 1)
	signal.Notify(channel, syscall.SIGINT, syscall.SIGTERM)
	<-channel
}

func server(cfg *Config) {
	ctx, err := NewServer(cfg)
	if err != nil {
		log.Printf("[server] %v", err)
		return
	}
	defer ctx.Close()

	channel := make(chan os.Signal, 1)
	signal.Notify(channel, syscall.SIGINT, syscall.SIGTERM)
	<-channel
}

func main() {
	data, err := os.ReadFile(os.Args[1])
	if err != nil {
		return
	}

	info := Config{}
	if err := yaml.Unmarshal(data, &info); err != nil {
		return
	}
	log.Printf("[main] %s", info.ID)

	switch info.Mode {
	case "client":
		client(&info)
	case "server":
		server(&info)
	}
}
