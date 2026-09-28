package hwinfo

import (
	"net"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

type Info struct {
	GPUName    string
	VRAMMb     int
	RAMMb      int
	DiskFreeGB int
	OverlayIP  string
}

func Collect() Info {
	info := Info{
		RAMMb:      estimateRAM(),
		DiskFreeGB: 100,
		OverlayIP:  firstTailscaleIP(),
	}
	name, vram := nvidia()
	info.GPUName = name
	info.VRAMMb = vram
	return info
}

func estimateRAM() int {
	// Best-effort; detailed syscalls differ by OS.
	switch runtime.GOOS {
	case "windows":
		out, err := exec.Command("wmic", "ComputerSystem", "get", "TotalPhysicalMemory", "/value").Output()
		if err == nil {
			for _, line := range strings.Split(string(out), "\n") {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "TotalPhysicalMemory=") {
					v := strings.TrimPrefix(line, "TotalPhysicalMemory=")
					n, _ := strconv.ParseInt(v, 10, 64)
					return int(n / (1024 * 1024))
				}
			}
		}
	case "linux":
		out, err := exec.Command("sh", "-c", "awk '/MemTotal/ {print $2}' /proc/meminfo").Output()
		if err == nil {
			n, _ := strconv.Atoi(strings.TrimSpace(string(out)))
			return n / 1024
		}
	}
	return 0
}

func nvidia() (string, int) {
	out, err := exec.Command("nvidia-smi", "--query-gpu=name,memory.total", "--format=csv,noheader,nounits").Output()
	if err != nil {
		return "", 0
	}
	line := strings.TrimSpace(strings.Split(string(out), "\n")[0])
	parts := strings.Split(line, ",")
	if len(parts) < 2 {
		return line, 0
	}
	name := strings.TrimSpace(parts[0])
	vram, _ := strconv.Atoi(strings.TrimSpace(parts[1]))
	return name, vram
}

func firstTailscaleIP() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	for _, iface := range ifaces {
		name := strings.ToLower(iface.Name)
		if !(strings.Contains(name, "tailscale") || strings.Contains(name, "ts")) {
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
	// Fallback: any 100.x CGNAT-like address
	addrs, _ := net.InterfaceAddrs()
	for _, a := range addrs {
		ipnet, ok := a.(*net.IPNet)
		if !ok || ipnet.IP.To4() == nil {
			continue
		}
		ip := ipnet.IP.To4()
		if ip[0] == 100 && ip[1] >= 64 && ip[1] <= 127 {
			return ip.String()
		}
	}
	return ""
}
