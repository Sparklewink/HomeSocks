package health

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
	"time"

	"golang.org/x/sys/unix"

	"homesocks/internal/config"
	"homesocks/internal/vpn"
)

var lastCurrentDelay int = -1
func StartHealthCheckLoop() {
	interval := time.Duration(config.GlobalConfig.HealthCheckInterval) * time.Second
	if interval < 30*time.Second {
		interval = 60 * time.Second 
	}
	ticker := time.NewTicker(interval)
	go runHealthCheck()

	for range ticker.C {
		runHealthCheck()
	}
}

func runHealthCheck() {
	state := config.GetState()
	if !state.MasterSwitch || !state.AutoTestEnabled {
		return
	}
	if state.RoutingMode == "route" {
		delay, err := checkCurrentTunnel()
		if err != nil {
			log.Printf("[Health] Current tunnel is DEAD or Timeout.")
			lastCurrentDelay = -1
			if state.NodeSelectMode == "auto" {
				log.Printf("[Health] Auto mode active. Searching for a new fallback node...")
				AutoSwitchNode()
			}
		} else {
			lastCurrentDelay = delay
			// log.Printf("[Health] Current tunnel delay: %d ms", delay)
		}
	}
}

func GetLastDelay() int {
	return lastCurrentDelay
}

func checkCurrentTunnel() (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	start := time.Now()
	conn, err := vpn.DialContextInNetns(ctx, "tcp", "1.1.1.1:443")
	if err != nil {
		return -1, err
	}
	defer conn.Close()
	return int(time.Since(start).Milliseconds()), nil
}
func AutoSwitchNode() {
	nodes := vpn.GetNodes()
	if len(nodes) == 0 {
		return
	}
	limit := 5
	if len(nodes) < 5 {
		limit = len(nodes)
	}

	for i := 0; i < limit; i++ {
		candidate := nodes[i]
		log.Printf("[Health] Testing candidate node: %s", candidate.IP)
		delay, ok := TestNodeRealConnection(candidate)
		if ok && delay > 0 {
			log.Printf("[Health] Candidate %s is working! Delay: %d ms. Switching...", candidate.IP, delay)
			err := vpn.StartTunnel(candidate)
			if err == nil {
				config.UpdateState(func(s *config.State) {
					s.CurrentNodeIP = candidate.IP
				})
				lastCurrentDelay = delay
				return
			}
		}
	}
}

func TestNodeRealConnection(node vpn.Node) (int, bool) {
	nsName := fmt.Sprintf("hs-test-%d", time.Now().UnixNano()%100000)
	exec.Command("ip", "netns", "add", nsName).Run()
	exec.Command("ip", "netns", "exec", nsName, "ip", "link", "set", "dev", "lo", "up").Run()
	defer exec.Command("ip", "netns", "del", nsName).Run()
	configData, err := base64.StdEncoding.DecodeString(node.OvpnBase64)
	if err != nil {
		return -1, false
	}
	tempFile := fmt.Sprintf("/tmp/%s.ovpn", nsName)
	ioutil.WriteFile(tempFile, configData, 0644)
	defer os.Remove(tempFile)
	ovpnCmd := exec.Command("ip", "netns", "exec", nsName, "openvpn", "--config", tempFile)
	if err := ovpnCmd.Start(); err != nil {
		return -1, false
	}
	defer func() {
		if ovpnCmd.Process != nil {
			ovpnCmd.Process.Kill()
			ovpnCmd.Wait()
		}
	}()
	time.Sleep(3500 * time.Millisecond)
	delay, err := dialInSpecificNetns(nsName, "1.1.1.1:443")
	if err != nil {
		return -1, false
	}
	return delay, true
}
func dialInSpecificNetns(nsName, address string) (int, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	hostNsFile, err := os.Open("/proc/thread-self/ns/net")
	if err != nil {
		return -1, err
	}
	defer hostNsFile.Close()

	targetNsPath := fmt.Sprintf("/var/run/netns/%s", nsName)
	targetNsFile, err := os.Open(targetNsPath)
	if err != nil {
		return -1, err
	}
	defer targetNsFile.Close()

	if err := unix.Setns(int(targetNsFile.Fd()), unix.CLONE_NEWNET); err != nil {
		return -1, err
	}
	defer func() {
		unix.Setns(int(hostNsFile.Fd()), unix.CLONE_NEWNET)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	start := time.Now()
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", address)
	if err != nil {
		return -1, err
	}
	conn.Close()

	return int(time.Since(start).Milliseconds()), nil
}