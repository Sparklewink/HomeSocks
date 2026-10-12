package vpn

import (
	"encoding/csv"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"homesocks/internal/config"
)

type Node struct {
	IP          string `json:"ip"`
	Country     string `json:"country"`
	Ping        string `json:"ping"`
	Speed       string `json:"speed"`
	OvpnBase64  string `json:"-"` 
	Delay       int    `json:"delay"` 
	IsAvailable bool   `json:"is_available"`
}

var (
	NodePool []Node
	poolLock sync.RWMutex
)

func StartNodeSyncLoop() {
	syncNodes()
	interval := time.Duration(config.GlobalConfig.NodeSync.IntervalHours) * time.Hour
	if interval == 0 {
		interval = 24 * time.Hour
	}
	ticker := time.NewTicker(interval)
	for range ticker.C {
		syncNodes()
	}
}

func syncNodes() {
	url := config.GlobalConfig.NodeSync.SourceURL
	log.Printf("[Sync] Fetching nodes from %s", url)

	resp, err := http.Get(url)
	if err != nil {
		log.Printf("[Sync] Fetch failed: %v", err)
		return
	}
	defer resp.Body.Close()

	reader := csv.NewReader(resp.Body)
	reader.FieldsPerRecord = -1 

	records, err := reader.ReadAll()
	if err != nil {
		log.Printf("[Sync] Parse CSV failed: %v", err)
		return
	}

	var newPool []Node
	for _, row := range records {
		if len(row) < 15 {
			continue
		}
		hostName := row[0]
		ip := row[1]
		countryShort := row[6]
		operator := row[12]
		ovpnBase64 := row[14]
		if strings.HasPrefix(hostName, "*") || strings.HasPrefix(hostName, "#") {
			continue
		}
		lowerOp := strings.ToLower(operator)
		lowerHost := strings.ToLower(hostName)
		if strings.Contains(lowerOp, "public-vpn") || strings.Contains(lowerHost, "public-vpn") {
			continue
		}
		if strings.HasPrefix(ip, "219.100.37.") {
			continue
		}
		if ovpnBase64 == "" {
			continue
		}

		newPool = append(newPool, Node{
			IP:          ip,
			Country:     countryShort,
			Ping:        row[3],
			Speed:       row[4],
			OvpnBase64:  ovpnBase64,
			Delay:       -1,
			IsAvailable: false,
		})
	}

	poolLock.Lock()
	NodePool = newPool
	poolLock.Unlock()

	log.Printf("[Sync] Load successful. Valid residential nodes: %d", len(newPool))
}

func GetNodes() []Node {
	poolLock.RLock()
	defer poolLock.RUnlock()
	nodesCopy := make([]Node, len(NodePool))
	copy(nodesCopy, NodePool)
	return nodesCopy
}
