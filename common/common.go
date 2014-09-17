package common

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
)

func LoadConfigs() (*Config, *EtcdConfig, *FleetConfig, []string) {
	argv1 := os.Args
	cfg, argv2, err := NewConfig(argv1)
	if err != nil {
		fmt.Printf("Error parsing main options: %s\n", err.Error())
		os.Exit(1)
	}
	etcd, argv3, err := NewEtcdConfig(argv2, cfg.UUID)
	if err != nil {
		fmt.Printf("Error parsing etcd options: %s\n", err.Error())
		os.Exit(1)
	}
	fleet, argv4, err := NewFleetConfig(argv3)
	if err != nil {
		fmt.Printf("Error parsing fleet options: %s\n", err.Error())
		os.Exit(1)
	}
	args := argv4

	return cfg, etcd, fleet, args
}

func LocalNetForIp(fromIP net.IP) (*net.Interface, net.Addr, net.IP, error) {
	if ifaces, err := net.Interfaces(); err != nil {
		return nil, nil, nil, fmt.Errorf("LocalNetForIp: Error getting interfaces: %s\n", err.Error())
	} else {
		for _, iface := range ifaces {
			if (iface.Flags & net.FlagLoopback) != 0 {
				continue
			}
			if iface_addrs, err := iface.Addrs(); err != nil {
				return nil, nil, nil, fmt.Errorf("LocalNetForIp: Error getting interface addresses: %s\n", err.Error())
			} else {
				for _, iface_addr := range iface_addrs {
					ipstr := iface_addr.String()
					ip, ipnet, err := net.ParseCIDR(ipstr)
					if err != nil {
						return nil, nil, nil,
							fmt.Errorf("LocalNetForIp: Error parsing local address '%s': %s\n",
								ipstr, err.Error())
					}
					//fmt.Println("LocalNetForIp", "iface", iface, "ifaceaddr", iface_addr, "ip", ip, "ipnet", ipnet, "from", from)
					//fromHost, _, err := net.SplitHostPort(from.String())
					//if err != nil {
					//	return nil, nil, nil,
					//		fmt.Errorf("LocalNetForIp: Error parsing from address '%s': %s\n",
					//			from.String(), err.Error())
					//}
					// split away the IPv6 zone if present
					//fromHost = strings.Split(fromHost, "%")[0]
					//fromIP := net.ParseIP(fromHost)
					//if fromIP == nil {
					//	return nil, nil, nil,
					//		fmt.Errorf("LocalNetForIp: Error parsing from host '%s'\n", fromHost)
					//}
					if ipnet.Contains(fromIP) {
						return &iface, iface_addr, ip, nil
					}
				}
			}
		}
	}
	return nil, nil, nil, fmt.Errorf("LocalNetForIp: Could not locate local interface/address matching '%s'", fromIP.String())
}

func InterfaceIsVirtual(iface *net.Interface) bool {
	// linux specific, but this is for CoreOS anyway...
	// Look at the interface symlink in /sys/ to see if it is a virtual device or not.
	sysPath := fmt.Sprintf("/sys/class/net/%s", iface.Name)
	if linkTarget, err := filepath.EvalSymlinks(sysPath); err != nil {
		// possibly not linux, be cautious.
	} else {
		return strings.Contains(linkTarget, "/virtual/")
	}
	return false
}
