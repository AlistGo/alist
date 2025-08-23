package utils

import "strings"

// MaskIP anonymizes middle segments of an IP address.
func MaskIP(ip string) string {
	if ip == "" {
		return ""
	}
	if strings.Contains(ip, ":") {
		parts := strings.Split(ip, ":")
		if len(parts) > 2 {
			for i := 1; i < len(parts)-1; i++ {
				if parts[i] != "" {
					parts[i] = "*"
				}
			}
			return strings.Join(parts, ":")
		}
		return ip
	}
	parts := strings.Split(ip, ".")
	if len(parts) == 4 {
		for i := 1; i < len(parts)-1; i++ {
			parts[i] = "*"
		}
		return strings.Join(parts, ".")
	}
	return ip
}
