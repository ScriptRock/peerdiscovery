package common

import (
	"fmt"
	go_flags "github.com/jessevdk/go-flags"
	"io/ioutil"
	"net"
	"strings"
)

type EtcdPeer struct {
	Interface *net.Interface
	LocalIP   net.IP
	PeerIP    net.IP
	PeerPort  int
}

type EtcdConfig struct {
	Name           string   `long:"etcd_name" description:"etcd machine name, must be unique within cluster. Default is UUID"`
	ConfPath       string   `long:"etcd_conf" description:"etcd conf path (default /etc/etcd/etcd.conf)"`
	ClientAddr     string   `long:"etcd_client_addr" description:"etcd client address (default from $private_ipv4, or peers)"`
	ClientBindAddr string   `long:"etcd_client_bind_addr" description:"etcd client bind address (default 0.0.0.0)"`
	ClientPort     int      `long:"etcd_client_port" description:"etcd client port (default 4001)"`
	PeerAddr       string   `long:"etcd_peer_addr" description:"etcd peer address (default 0.0.0.0)"`
	PeerBindAddr   string   `long:"etcd_peer_bind_addr" description:"etcd peer bind address (default 0.0.0.0)"`
	PeerPort       int      `long:"etcd_peer_port" description:"etcd peer port (default 7001)"`
	DiscoveryURL   string   `long:"etcd_discovery_url" description:"etcd peer discovery url"`
	Peers          []string // found through mDNS etc
	ServerPeers    map[string]EtcdPeer
	BootingPeers   map[string]EtcdPeer
	AddrSource     string `long:"addr_from" description:"where to obtain addr & peer_addr from. Options: private_ipv4, public_ipv4, or heuristics"`
	Interface      *net.Interface
}

func (c *EtcdConfig) load(argsin []string, name string) ([]string, error) {
	// Set some defaults
	c.Name = name
	c.ConfPath = "/etc/etcd/etcd.conf"
	c.ClientAddr = "" // there is much more complex logic around this elsewhere
	c.ClientPort = 4001
	c.PeerAddr = ""
	c.ClientBindAddr = "0.0.0.0"
	c.PeerBindAddr = "0.0.0.0"
	c.PeerPort = 7001
	c.DiscoveryURL = ""
	c.Peers = make([]string, 0)
	c.ServerPeers = make(map[string]EtcdPeer)
	c.BootingPeers = make(map[string]EtcdPeer)
	c.AddrSource = "/etc/private_ipv4"

	// TODO FIXME one day, load some from an incoming config file
	// TODO FIXME one day, override defaults and incoming conf with environment variables

	// override defaults, incoming conf, env vars with command line arguments
	argsout, err := go_flags.NewParser(c, go_flags.IgnoreUnknown).ParseArgs(argsin)

	// Warning: go_flags parser will set pointer types to not nil.... bad.
	c.Interface = nil

	// load client address from /etc/ if present
	if true && c.ClientAddr == "" {
		if useAddr, err := ioutil.ReadFile(c.AddrSource); err == nil {
			if ip := net.ParseIP(strings.TrimSpace(string(useAddr))); ip == nil {
				fmt.Printf("Invalid IP address found in '%s'\n", c.AddrSource)
			} else {
				c.verifyIP(ip, c.AddrSource)
			}
		}
	}

	return argsout, err
}

func (c *EtcdConfig) AddServerPeer(iface *net.Interface, localIP net.IP, peerIP net.IP, peerPort int) {
	c.ServerPeers[peerIP.String()] = EtcdPeer{
		Interface: iface,
		LocalIP:   localIP,
		PeerIP:    peerIP,
		PeerPort:  peerPort,
	}
}

func (c *EtcdConfig) AddBootingPeer(iface *net.Interface, localIP net.IP, peerIP net.IP, peerPort int) {
	c.BootingPeers[peerIP.String()] = EtcdPeer{
		Interface: iface,
		LocalIP:   localIP,
		PeerIP:    peerIP,
		PeerPort:  peerPort,
	}
}

func (c *EtcdConfig) verifyIP(ip net.IP, source string) {
	fmt.Printf("IP '%s' found in '%s'\n", ip.String(), source)
	// valid ip address found from source. Verify that it exists
	if iface, ip_net, ipverify, err := LocalNetForIp(ip); err != nil {
		fmt.Printf("%s\n", err.Error())
	} else if !ip.Equal(ipverify) {
		fmt.Printf("IP address on interface %s: %s does not match ip from %s: %s\n",
			iface.Name, ipverify, c.AddrSource, ip)
	} else {
		fmt.Printf("IP '%s' verified, on interface '%s' net '%s'\n",
			ip.String(), iface.Name, ip_net.String())
		set := false
		// set local defaults appropriately
		if c.ClientAddr == "" {
			c.ClientAddr = ip.String()
			set = true
		}
		if c.PeerAddr == "" {
			c.PeerAddr = ip.String()
			set = true
		}
		if set {
			c.Interface = iface
		}
	}
}

func IsIPv4(ip net.IP) bool {
	return ip.DefaultMask() != nil
}

func (c *EtcdConfig) setupAddresses() {
	// Now that all load sources have been tested; set up local addresses for etcd config

	// If a peer was found, use our local address based on that
	if c.ClientAddr == "" {
		for _, v := range c.ServerPeers {
			c.ClientAddr = v.LocalIP.String()
			fmt.Printf("setupAddresses: heuristic client address from server peer: %s\n", c.ClientAddr)
			break
		}
	}

	// Otherwise use booting peer; we may be the first to boot
	if c.ClientAddr == "" {
		for _, v := range c.BootingPeers {
			c.ClientAddr = v.LocalIP.String()
			fmt.Printf("setupAddresses: heuristic client address from booting peer: %s\n", c.ClientAddr)
			break
		}
	}

	// Last resort; iterate over our interfaces and use the last non-virtual one (linux specific)
	if c.ClientAddr == "" {
		var lastIP net.IP = nil
		if ifaces, err := net.Interfaces(); err != nil {
			fmt.Printf("setupAddresses: Error getting network interfaces: '%s'\n", err.Error())
		} else {
			for _, iface := range ifaces {
				if (iface.Flags & net.FlagLoopback) != 0 {
					continue
				}
				if (iface.Flags & net.FlagUp) == 0 {
					continue
				}
				if (iface.Flags & net.FlagMulticast) == 0 {
					continue
				}
				if InterfaceIsVirtual(&iface) {
					continue
				}

				if iface_addrs, err := iface.Addrs(); err != nil {
					fmt.Errorf("setupAddresses: Error getting interface addresses: %s\n", err.Error())
				} else {
					for _, iface_addr := range iface_addrs {
						ipstr := iface_addr.String()
						ip, _, err := net.ParseCIDR(ipstr)
						if err != nil {
							fmt.Printf("Error parsing local address '%s': %s\n",
								ipstr, err.Error())
						} else {
							// Use an IPv4 address only
							if IsIPv4(ip) {
								lastIP = ip
							}
						}
					}
				}
			}
		}
		if lastIP != nil {
			c.ClientAddr = lastIP.String()
			fmt.Printf("setupAddresses: heuristic client address from last network interface: %s\n", c.ClientAddr)
		} else {
			c.ClientAddr = "127.0.0.1"
			fmt.Printf("setupAddresses: cannot find any valid addresses; using loopback interface: %s\n", c.ClientAddr)
		}
	}

	if c.PeerAddr == "" {
		c.PeerAddr = c.ClientAddr
	}
}

func NewEtcdConfig(argsin []string, name string) (*EtcdConfig, []string, error) {
	c := new(EtcdConfig)
	argsout, err := c.load(argsin, name)
	return c, argsout, err
}

func (cfg *EtcdConfig) WriteFile() {
	cfg.setupAddresses()

	peers := make([]string, 0)
	if cfg.DiscoveryURL == "" {
		for k, _ := range cfg.ServerPeers {
			peers = append(peers, fmt.Sprintf("\"%s:%d\"", k, cfg.PeerPort))
		}
	}

	// wrap each peer in quotes
	conf := fmt.Sprintf(
		`
#
# Generated by ScriptRock Config init
#
name = "%s"
addr = "%s:%d"
bind_addr = "%s:%d"
#ca_file = ""
#cert_file = ""
#cors = []
#cpu_profile_file = ""
#data_dir = "."
discovery = "%s"
#http_read_timeout = 10.0
#http_write_timeout = 10.0
#key_file = ""
peers = [%s]
#peers_file = ""
#max_cluster_size = 9
#max_result_buffer = 1024
#max_retry_attempts = 3
#snapshot = true
#verbose = false
#very_verbose = false
#
[peer]
addr = "%s:%d"
bind_addr = "%s:%d"
#ca_file = ""
#cert_file = ""
#key_file = ""
#
#[cluster]
#active_size = 9
#remove_delay = 1800.0
#sync_interval = 5.0
#
`,
		cfg.Name,                       // name
		cfg.ClientAddr, cfg.ClientPort, // addr
		cfg.ClientBindAddr, cfg.ClientPort, // bind_addr
		cfg.DiscoveryURL,           // discovery
		strings.Join(peers, ","),   // peers
		cfg.PeerAddr, cfg.PeerPort, // peer_addr
		cfg.PeerBindAddr, cfg.PeerPort) // peer_bind_addr

	fmt.Printf("Writing etcd conf file to '%s'\n", cfg.ConfPath)
	if err := ioutil.WriteFile(cfg.ConfPath, []byte(conf), 0644); err != nil {
		fmt.Printf("Could not write conf file '%s': %s\n", cfg.ConfPath, err.Error())
	}
}
