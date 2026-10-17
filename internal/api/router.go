package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	"homesocks/internal/config"
	"homesocks/internal/health"
	"homesocks/internal/vpn"
	"homesocks/web"
)

func StartWebServer() {
	cfg := config.GlobalConfig
	mux := http.NewServeMux()

	prefix := cfg.PanelPath
	if !strings.HasPrefix(prefix, "/") {
		prefix = "/" + prefix
	}

	fileServer := http.FileServer(http.FS(web.FS))
	mux.Handle(prefix+"/", http.StripPrefix(prefix, fileServer))

	mux.Handle(prefix+"/api/status", authMiddleware(http.HandlerFunc(statusHandler)))
	mux.Handle(prefix+"/api/nodes", authMiddleware(http.HandlerFunc(nodesHandler)))
	mux.Handle(prefix+"/api/action", authMiddleware(http.HandlerFunc(actionHandler)))
	mux.Handle(prefix+"/api/test", authMiddleware(http.HandlerFunc(testHandler)))

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})

	addr := fmt.Sprintf(":%d", cfg.PanelPort)
	log.Printf("[Web] Panel listening on http://0.0.0.0%s%s/", addr, prefix)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("[Web] Server failed: %v", err)
	}
}

func authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("Authorization")
		expected := "Bearer " + config.GlobalConfig.PanelPassword
		if token != expected {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func statusHandler(w http.ResponseWriter, r *http.Request) {
	state := config.GetState()
	cfg := config.GlobalConfig

	resp := map[string]interface{}{
		"state": state,
		"config": map[string]interface{}{
			"socks_port": cfg.SocksPort,
			"socks_auth": cfg.SocksAuth.Enabled,
		},
		"delay": health.GetLastDelay(),
	}
	json.NewEncoder(w).Encode(resp)
}

func nodesHandler(w http.ResponseWriter, r *http.Request) {
	nodes := vpn.GetNodes()
	for i := range nodes {
		nodes[i].OvpnBase64 = "" 
	}
	json.NewEncoder(w).Encode(nodes)
}

func actionHandler(w http.ResponseWriter, r *http.Request) {
	var req map[string]interface{}
	json.NewDecoder(r.Body).Decode(&req)

	action, _ := req["action"].(string)

	config.UpdateState(func(s *config.State) {
		switch action {
		case "toggle_master":
			s.MasterSwitch = !s.MasterSwitch
		case "toggle_routing":
			if s.RoutingMode == "route" {
				s.RoutingMode = "direct"
			} else {
				s.RoutingMode = "route"
			}
		case "set_auto":
			s.NodeSelectMode = "auto"
			s.AutoTestEnabled = true
			go health.AutoSwitchNode()
		case "set_manual":
			ip, _ := req["ip"].(string)
			s.NodeSelectMode = "manual"
			s.CurrentNodeIP = ip
			nodes := vpn.GetNodes()
			for _, n := range nodes {
				if n.IP == ip {
					go vpn.StartTunnel(n)
					break
				}
			}
		case "update_priority":
			pStr, _ := req["priority"].(string)
			parts := strings.Split(pStr, ",")
			var clean []string
			for _, p := range parts {
				trimP := strings.ToUpper(strings.TrimSpace(p))
				if trimP != "" {
					clean = append(clean, trimP)
				}
			}
			s.PriorityCountries = clean
		}
	})

	w.WriteHeader(http.StatusOK)
}

func testHandler(w http.ResponseWriter, r *http.Request) {
	var req map[string]string
	json.NewDecoder(r.Body).Decode(&req)
	targetIP := req["ip"]

	var targetNode vpn.Node
	found := false
	for _, n := range vpn.GetNodes() {
		if n.IP == targetIP {
			targetNode = n
			found = true
			break
		}
	}

	if !found {
		http.Error(w, "Node not found", 404)
		return
	}

	delay, ok := health.TestNodeRealConnection(targetNode)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"ip":    targetIP,
		"delay": delay,
		"ok":    ok,
	})
}