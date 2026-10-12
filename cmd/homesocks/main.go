package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"homesocks/internal/api"
	"homesocks/internal/config"
	"homesocks/internal/health"
	"homesocks/internal/socks5"
	"homesocks/internal/vpn"
)

func main() {
	configPath := flag.String("c", "/etc/homesocks/config.json", "Path to config.json")
	flag.Parse()

	log.Println("=== Starting HomeSocks ===")
	err := config.Init(*configPath)
	if err != nil {
		log.Fatalf("Failed to init config: %v", err)
	}
	err = vpn.InitNetns()
	if err != nil {
		log.Fatalf("Failed to initialize netns: %v", err)
	}
	defer vpn.CleanupNetns()
	go vpn.StartNodeSyncLoop()
	go health.StartHealthCheckLoop()
	go socks5.StartServer()
	go api.StartWebServer()
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	log.Println("Shutting down HomeSocks...")
}