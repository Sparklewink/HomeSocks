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
	"strings"
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
				log.Printf("[Health] AUTO mode active. Searching for a new fallback node...")
				AutoSwitchNode()
			} else {
				log.Printf("[Health] MANUAL mode locked on %s. Will NOT auto-switch. Attempting to reconnect...", state.CurrentNodeIP)
				reconnectCurrentManualNode(state.CurrentNodeIP)
			}
		} else {
			lastCurrentDelay = delay
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

func reconnectCurrentManualNode(ip string) {
	nodes := vpn.GetNodes()
	for _, n := range nodes {
		if n.IP == ip {
			vpn.StartTunnel(n)
			break
		}
	}
}

func AutoSwitchNode() {
	nodes := vpn.GetNodes()
	if len(nodes) == 0 {
		return
	}

	state := config.GetState()
	var candidates []vpn.Node

	for _, pc := range state.PriorityCountries {
		pc = strings.ToUpper(strings.TrimSpace(pc))
		for _, n := range nodes {
			if strings.ToUpper(n.Country) == pc {
				candidates = append(candidates, n)
			}
		}
	}

	for _, n := range nodes {
		isPriority := false
		for _, pc := range state.PriorityCountries {
			if strings.ToUpper(n.Country) == strings.ToUpper(strings.TrimSpace(pc)) {
				isPriority = true
				break
			}
		}
		if !isPriority {
			candidates = append(candidates, n)
		}
	}

	limit := 10
	if len(candidates) < limit {
		limit = len(candidates)
	}

	for i := 0; i < limit; i++ {
		candidate := candidates[i]

		proto, addr := vpn.ExtractRemoteInfo(candidate.OvpnBase64)
		if proto == "tcp" && addr != "" {
			if !tcpShallowCheck(addr) {
				log.Printf("[Health] Shallow TCP test failed for %s. Skipping heavy test.", addr)
				continue 
			}
		}

		log.Printf("[Health] Deep testing candidate node: %s (%s)", candidate.IP, candidate.Country)
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

func tcpShallowCheck(addr string) bool {
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
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