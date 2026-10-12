package socks5

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"strconv"
	"time"

	"homesocks/internal/config"
	"homesocks/internal/vpn"
)

func StartServer() {
	cfg := config.GlobalConfig
	addr := fmt.Sprintf("%s:%d", cfg.SocksListen, cfg.SocksPort)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("[SOCKS5] Failed to listen on %s: %v", addr, err)
	}
	log.Printf("[SOCKS5] Listening on %s", addr)

	for {
		conn, err := listener.Accept()
		if err != nil {
			continue
		}
		go handleConnection(conn)
	}
}

func handleConnection(client net.Conn) {
	defer client.Close()

	state := config.GetState()
	if !state.MasterSwitch {
		return
	}
	if err := authenticate(client); err != nil {
		return
	}
	target, err := readRequest(client)
	if err != nil {
		return
	}
	var remote net.Conn
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if state.RoutingMode == "route" {
		remote, err = vpn.DialContextInNetns(ctx, "tcp", target)
	} else {
		var d net.Dialer
		remote, err = d.DialContext(ctx, "tcp", target)
	}

	if err != nil {
		client.Write([]byte{0x05, 0x05, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		return
	}
	defer remote.Close()
	client.Write([]byte{0x05, 0x00, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
	errChan := make(chan error, 2)
	go func() {
		_, err := io.Copy(remote, client)
		errChan <- err
	}()
	go func() {
		_, err := io.Copy(client, remote)
		errChan <- err
	}()
	<-errChan
}

func authenticate(conn net.Conn) error {
	buf := make([]byte, 256)
	if _, err := io.ReadFull(conn, buf[:2]); err != nil {
		return err
	}
	if buf[0] != 0x05 {
		return fmt.Errorf("unsupported SOCKS version")
	}

	numMethods := int(buf[1])
	if _, err := io.ReadFull(conn, buf[:numMethods]); err != nil {
		return err
	}

	authCfg := config.GlobalConfig.SocksAuth
	if !authCfg.Enabled {
		// No Auth
		conn.Write([]byte{0x05, 0x00})
		return nil
	}

	conn.Write([]byte{0x05, 0x02})
	if _, err := io.ReadFull(conn, buf[:2]); err != nil {
		return err
	}
	if buf[0] != 0x01 {
		return fmt.Errorf("unsupported auth version")
	}

	uLen := int(buf[1])
	if _, err := io.ReadFull(conn, buf[:uLen]); err != nil {
		return err
	}
	username := string(buf[:uLen])

	if _, err := io.ReadFull(conn, buf[:1]); err != nil {
		return err
	}
	pLen := int(buf[0])
	if _, err := io.ReadFull(conn, buf[:pLen]); err != nil {
		return err
	}
	password := string(buf[:pLen])

	if username == authCfg.Username && password == authCfg.Password {
		conn.Write([]byte{0x01, 0x00}) // Auth Success
		return nil
	}

	conn.Write([]byte{0x01, 0x01}) // Auth Failed
	return fmt.Errorf("invalid credentials")
}

func readRequest(conn net.Conn) (string, error) {
	buf := make([]byte, 256)
	if _, err := io.ReadFull(conn, buf[:4]); err != nil {
		return "", err
	}
	if buf[0] != 0x05 {
		return "", fmt.Errorf("unsupported version")
	}
	if buf[1] != 0x01 {
		conn.Write([]byte{0x05, 0x07, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		return "", fmt.Errorf("unsupported command (only TCP CONNECT is supported in MVP)")
	}

	var host string
	switch buf[3] {
	case 0x01: // IPv4
		if _, err := io.ReadFull(conn, buf[:4]); err != nil {
			return "", err
		}
		host = net.IP(buf[:4]).String()
	case 0x03: // Domain
		if _, err := io.ReadFull(conn, buf[:1]); err != nil {
			return "", err
		}
		dLen := int(buf[0])
		if _, err := io.ReadFull(conn, buf[:dLen]); err != nil {
			return "", err
		}
		host = string(buf[:dLen])
	case 0x04: // IPv6
		if _, err := io.ReadFull(conn, buf[:16]); err != nil {
			return "", err
		}
		host = net.IP(buf[:16]).String()
	}

	if _, err := io.ReadFull(conn, buf[:2]); err != nil {
		return "", err
	}
	port := binary.BigEndian.Uint16(buf[:2])

	return net.JoinHostPort(host, strconv.Itoa(int(port))), nil
}