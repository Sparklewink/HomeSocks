package vpn

import (
	"encoding/base64"
	"strings"
)

func ExtractRemoteInfo(ovpnBase64 string) (string, string) {
	data, err := base64.StdEncoding.DecodeString(ovpnBase64)
	if err != nil {
		return "", ""
	}

	lines := strings.Split(string(data), "\n")
	proto := "udp"
	addr := ""

	for _, line := range lines {
		line = strings.TrimSpace(line)
		lowerLine := strings.ToLower(line)

		if strings.HasPrefix(lowerLine, "proto ") {
			parts := strings.Fields(lowerLine)
			if len(parts) >= 2 {
				if strings.Contains(parts[1], "tcp") {
					proto = "tcp"
				} else if strings.Contains(parts[1], "udp") {
					proto = "udp"
				}
			}
		}

		if strings.HasPrefix(lowerLine, "remote ") {
			parts := strings.Fields(lowerLine)
			if len(parts) >= 3 {
				addr = parts[1] + ":" + parts[2]
			}
		}
	}
	return proto, addr
}