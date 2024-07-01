package schedule

import (
	"encoding/base64"
	"net"
)

// Base64Encode base64编码
func Base64Encode(rawBytes []byte) string {
	return base64.StdEncoding.EncodeToString(rawBytes)
}

// GetLocalIP 获取本地ip地址（单播地址）
func GetLocalIP() (string, error) {
	addrList, err := net.InterfaceAddrs()
	if err != nil {
		return "", err
	}

	for _, addr := range addrList {
		if ipNet, ok := addr.(*net.IPNet); ok && ipNet.IP.IsGlobalUnicast() {
			if ipNet.IP.To4() != nil {
				return ipNet.IP.String(), nil
			}
		}
	}

	return "", nil
}
