package config

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"sync"
)

type Config struct {
	PanelPort           int       `json:"panel_port"`
	PanelPath           string    `json:"panel_path"`
	PanelPassword       string    `json:"panel_password"`
	SocksPort           int       `json:"socks_port"`
	SocksListen         string    `json:"socks_listen"`
	SocksAuth           SocksAuth `json:"socks_auth"`
	HealthCheckInterval int       `json:"health_check_interval"`
	NodeSync            NodeSync  `json:"node_sync"`
}

type SocksAuth struct {
	Enabled  bool   `json:"enabled"`
	Username string `json:"username"`
	Password string `json:"password"`
}

type NodeSync struct {
	SourceURL     string `json:"source_url"`
	IntervalHours int    `json:"interval_hours"`
}

type State struct {
	MasterSwitch      bool     `json:"master_switch"`
	RoutingMode       string   `json:"routing_mode"`     // "route" 或 "direct"
	NodeSelectMode    string   `json:"node_select_mode"` // "auto" 或 "manual"
	AutoTestEnabled   bool     `json:"auto_test_enabled"`
	CurrentNodeIP     string   `json:"current_node_ip"`
	PriorityCountries []string `json:"priority_countries"` // 自动模式下的国家优先级，如 ["JP", "KR", "US"]
}

var (
	GlobalConfig Config
	GlobalState  State
	stateMutex   sync.RWMutex
	configDir    string
)

func Init(path string) error {
	configDir = filepath.Dir(path)

	data, err := ioutil.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, &GlobalConfig); err != nil {
		return err
	}

	statePath := filepath.Join(configDir, "state.json")
	if _, err := os.Stat(statePath); os.IsNotExist(err) {
		// 初始化默认状态
		GlobalState = State{
			MasterSwitch:      true,
			RoutingMode:       "route",
			NodeSelectMode:    "auto",
			AutoTestEnabled:   true,
			PriorityCountries: []string{"JP", "KR", "US"}, // 默认优先国家
		}
		SaveState()
	} else {
		sData, _ := ioutil.ReadFile(statePath)
		json.Unmarshal(sData, &GlobalState)
	}

	log.Printf("Config loaded: PanelPort=%d, Mode=%s, Priority=%v", GlobalConfig.PanelPort, GlobalState.NodeSelectMode, GlobalState.PriorityCountries)
	return nil
}

func SaveState() error {
	stateMutex.Lock()
	defer stateMutex.Unlock()

	data, err := json.MarshalIndent(GlobalState, "", "  ")
	if err != nil {
		return err
	}
	statePath := filepath.Join(configDir, "state.json")
	return ioutil.WriteFile(statePath, data, 0644)
}

func UpdateState(mutate func(*State)) {
	stateMutex.Lock()
	mutate(&GlobalState)
	stateMutex.Unlock()
	SaveState()
}

func GetState() State {
	stateMutex.RLock()
	defer stateMutex.RUnlock()
	return GlobalState
}