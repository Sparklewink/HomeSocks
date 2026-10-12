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
	HealthCheckInterval int       `json:"health_check_interval"` // 秒
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
	MasterSwitch    bool   `json:"master_switch"`
	RoutingMode     string `json:"routing_mode"` 
	NodeSelectMode  string `json:"node_select_mode"`
	AutoTestEnabled bool   `json:"auto_test_enabled"`
	CurrentNodeIP   string `json:"current_node_ip"`
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
		GlobalState = State{
			MasterSwitch:    true,
			RoutingMode:     "route",
			NodeSelectMode:  "auto",
			AutoTestEnabled: true,
		}
		SaveState()
	} else {
		sData, _ := ioutil.ReadFile(statePath)
		json.Unmarshal(sData, &GlobalState)
	}

	log.Printf("Config loaded: PanelPort=%d, RoutingMode=%s", GlobalConfig.PanelPort, GlobalState.RoutingMode)
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