package vpn

import (
	"context"
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"time"

	"golang.org/x/sys/unix"
)

const NetnsName = "hs-netns"

var (
	ovpnCmd   *exec.Cmd
	ovpnMutex sync.Mutex
)

func InitNetns() error {
	exec.Command("ip", "netns", "del", NetnsName).Run()

	cmd := exec.Command("ip", "netns", "add", NetnsName)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ip netns add failed: %v", err)
	}

	cmd = exec.Command("ip", "netns", "exec", NetnsName, "ip", "link", "set", "dev", "lo", "up")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("set lo up failed: %v", err)
	}

	log.Printf("[NetNS] Namespace '%s' initialized.", NetnsName)
	return nil
}

func CleanupNetns() {
	StopTunnel()
	exec.Command("ip", "netns", "del", NetnsName).Run()
	log.Printf("[NetNS] Namespace '%s' destroyed.", NetnsName)
}

func StartTunnel(node Node) error {
	ovpnMutex.Lock()
	defer ovpnMutex.Unlock()

	if ovpnCmd != nil && ovpnCmd.Process != nil {
		ovpnCmd.Process.Kill()
		ovpnCmd.Wait()
		ovpnCmd = nil
	}

	configData, err := base64.StdEncoding.DecodeString(node.OvpnBase64)
	if err != nil {
		return fmt.Errorf("decode ovpn base64 failed: %v", err)
	}

	tempFile := "/etc/homesocks/current.ovpn"
	ioutil.WriteFile(tempFile, configData, 0644)

	ovpnCmd = exec.Command("ip", "netns", "exec", NetnsName, "openvpn",
		"--config", tempFile,
		"--auth-nocache",
		"--daemon",
	)

	err = ovpnCmd.Start()
	if err != nil {
		return fmt.Errorf("start openvpn failed: %v", err)
	}

	time.Sleep(3 * time.Second)
	log.Printf("[NetNS] Tunnel started for IP: %s", node.IP)
	return nil
}
func StopTunnel() {
	ovpnMutex.Lock()
	defer ovpnMutex.Unlock()
	if ovpnCmd != nil && ovpnCmd.Process != nil {
		ovpnCmd.Process.Kill()
		ovpnCmd.Wait()
		ovpnCmd = nil
		log.Println("[NetNS] Tunnel stopped.")
	}
}

func DialContextInNetns(ctx context.Context, network, address string) (net.Conn, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	hostNsFile, err := os.Open("/proc/thread-self/ns/net")
	if err != nil {
		return nil, fmt.Errorf("open host netns failed: %v", err)
	}
	defer hostNsFile.Close()
	targetNsPath := fmt.Sprintf("/var/run/netns/%s", NetnsName)
	targetNsFile, err := os.Open(targetNsPath)
	if err != nil {
		return nil, fmt.Errorf("open target netns %s failed: %v", NetnsName, err)
	}
	defer targetNsFile.Close()
	if err := unix.Setns(int(targetNsFile.Fd()), unix.CLONE_NEWNET); err != nil {
		return nil, fmt.Errorf("enter netns failed: %v", err)
	}
	defer func() {
		unix.Setns(int(hostNsFile.Fd()), unix.CLONE_NEWNET)
	}()
	var d net.Dialer
	return d.DialContext(ctx, network, address)
}