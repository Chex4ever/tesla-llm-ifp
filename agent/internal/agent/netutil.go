package agent

import (
	"fmt"
	"net"
	"strings"
)

func PrimaryLANIP() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return "127.0.0.1"
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := iface.Addrs()
		for _, a := range addrs {
			ipnet, ok := a.(*net.IPNet)
			if !ok || ipnet.IP.To4() == nil {
				continue
			}
			ip := ipnet.IP.To4()
			if ip[0] == 10 || (ip[0] == 192 && ip[1] == 168) || (ip[0] == 172 && ip[1] >= 16 && ip[1] <= 31) {
				return ip.String()
			}
		}
	}
	// any non-loopback
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := iface.Addrs()
		for _, a := range addrs {
			ipnet, ok := a.(*net.IPNet)
			if !ok || ipnet.IP.To4() == nil {
				continue
			}
			return ipnet.IP.String()
		}
	}
	return "127.0.0.1"
}

func NestListenURL(listenAddr, advertHost string) (listen string, wsURL string) {
	listen = listenAddr
	if listen == "" {
		listen = ":7843"
	}
	host := advertHost
	if host == "" {
		host = PrimaryLANIP()
	}
	port := "7843"
	if strings.HasPrefix(listen, ":") {
		port = strings.TrimPrefix(listen, ":")
	} else if _, p, err := net.SplitHostPort(listen); err == nil && p != "" {
		port = p
	}
	wsURL = fmt.Sprintf("ws://%s:%s/nest", host, port)
	return listen, wsURL
}
