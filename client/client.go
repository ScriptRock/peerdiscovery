package client

/*

Local peer discovery client

By Mark Sheahan, ScriptRock Inc

-- Polls local daemon to obtain the current peers list
-- Does UDP broadcast to find local peers interested in a particular service.
-- Used for etcd auto-clustering where there is no global discovery service available, such as behind firewalls.

*/

import (
	"fmt"
	"github.com/ScriptRock/mdns"
	"github.com/ScriptRock/peerdiscovery/common"
	"net"
	"net/http"
	"strings"
	//"sort"
	"os"
	"time"
)

func boolStr(b bool) string {
	if b {
		return "true"
	} else {
		return "false"
	}
}

/*
func pollServer(cfg *common.Config) (error) {
	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/%s", cfg.QueryPort, cfg.Group))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var all []common.PeerReport
	if err := json.Unmarshal(body, &all); err != nil {
		return nil, err
	} else {
		return all, nil
	}
}
*/

type ClientState struct {
	cfg                   *common.Config
	etcd                  *common.EtcdConfig
	mdnsBootupEntries     chan *mdns.ServiceEntry
	mdnsPeerServerEntries chan *mdns.ServiceEntry
	pollEvent             chan int
}

func newClientState(cfg *common.Config, etcd *common.EtcdConfig) *ClientState {
	return &ClientState{
		cfg:                   cfg,
		etcd:                  etcd,
		mdnsBootupEntries:     make(chan *mdns.ServiceEntry),
		mdnsPeerServerEntries: make(chan *mdns.ServiceEntry),
		pollEvent:             make(chan int),
	}
}

func mdnsEtcdBootupServer(cfg *common.Config, etcd *common.EtcdConfig) {
	service := &mdns.MDNSService{
		Instance: cfg.MDNSInstance,
		Service:  cfg.MDNSBootupService,
		Domain:   cfg.MDNSDomain,
		Port:     etcd.PeerPort,
	}
	if err := service.Init(); err != nil {
		fmt.Println("mDNS error", err)
	}

	mdns.NewServer(&mdns.Config{Zone: service})
}

func (cs *ClientState) pollInterface(iface *net.Interface) {
	bootParams := mdns.DefaultParams(cs.cfg.MDNSService)
	bootParams.Entries = cs.mdnsPeerServerEntries
	bootParams.Interface = iface

	etcdParams := mdns.DefaultParams(cs.cfg.MDNSBootupService)
	etcdParams.Entries = cs.mdnsBootupEntries
	etcdParams.Interface = iface

	mdns.Query(bootParams)
	mdns.Query(etcdParams)
}

func (cs *ClientState) pollLoop() {
	for {
		if cs.etcd.Interface != nil {
			cs.pollInterface(cs.etcd.Interface)
		} else {
			if ifaces, err := net.Interfaces(); err != nil {
				fmt.Printf("Error getting network interfaces: '%s'\n", err.Error())
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

					cs.pollInterface(&iface)
				}
			}
		}
		cs.pollEvent <- 0
		time.Sleep(cs.cfg.PollInterval)
	}
}

func (cs *ClientState) checkEnt(ent *mdns.ServiceEntry) (*net.Interface, net.IP, net.IP, error) {
	peerIP := ent.AddrV4
	if strings.HasPrefix(ent.Name, cs.cfg.UUID) {
		return nil, nil, nil, fmt.Errorf("Prefix UUID is from self")
	} else if peerIP == nil {
		return nil, nil, nil, fmt.Errorf("No IPv4 address present")
	}
	// This technically restricts valid configurations that go through routers. Will need to re-visit.
	// The objective is to prevent NATs where the return path will not work, not routers where the return path will.
	// Eventually, must set up a tcp/http server on the destination host that echoes back the connecting IP address,
	// and compare against that.
	iface, _, myIP, err := common.LocalNetForIp(ent.AddrV4)
	if err == nil && myIP.Equal(peerIP) {
		return nil, nil, nil, fmt.Errorf("IP address is self (%s = %s)", myIP.String(), peerIP.String())
	}
	return iface, myIP, peerIP, err
}

func (cs *ClientState) peerMDNSHostname(ent *mdns.ServiceEntry) string {
	return strings.Split(ent.Name, ".")[0] + "." + cs.cfg.MDNSDomain
}

func (cs *ClientState) stateTask() {
	polls := 0
	lastPollWithHigherPeer := 0
	finished := false

	for !finished {
		select {
		case <-cs.pollEvent:
			// time to give up and write out a conf
			polls = polls + 1
			fmt.Printf("poll occurred\n")
			if polls >= lastPollWithHigherPeer+cs.cfg.MaxLoops {
				fmt.Printf("%d consecutive polls with no lower peer; exiting\n", cs.cfg.MaxLoops)
				finished = true
			}
		case ent := <-cs.mdnsPeerServerEntries:
			// peer etcd server is apparently up...
			// do an HTTP request to the server to see if it truly exists
			if iface, localIP, peerIP, err := cs.checkEnt(ent); err != nil {
				//fmt.Println("etcd server", ent, "invalid", err)
			} else {
				peerMDNSHostname := cs.peerMDNSHostname(ent)
				fmt.Printf("etcd server mDNS response: IP %s mDNS hostname %s\n", peerIP.String(), peerMDNSHostname)
				url := fmt.Sprintf("http://%s:%d/v2/keys/", peerMDNSHostname, cs.etcd.ClientPort)
				if _, err := http.Get(url); err != nil {
					fmt.Printf("Error validating peer etcd server at '%s': %s\n", url, err.Error())
				} else {
					fmt.Printf("Peer etcd server found on %s (%s); exiting\n", url, peerIP)
					cs.etcd.AddServerPeer(iface, localIP, peerIP)
					finished = true
				}
			}
		case ent := <-cs.mdnsBootupEntries:
			// A peer whom is also booting exist. If it has a higher ip address
			// than us, wait until it no longer exists; it should start the etcd server first
			if iface, localIP, peerIP, err := cs.checkEnt(ent); err != nil {
				//fmt.Println("etcd bootpeer", ent, "invalid", err)
			} else {
				fmt.Println("Peer also booting:", ent)
				cs.etcd.AddBootingPeer(iface, localIP, peerIP)
				if localIP.String() < peerIP.String() {
					// we are lower; keep going
				} else {
					lastPollWithHigherPeer = polls
				}
			}
		}
	}
}

func Client() {
	cfg, etcd, args := common.LoadConfigs()
	if len(args) > 1 {
		fmt.Printf("Error parsing options; un-parsed options remain: %s\n", strings.Join(args[1:], ", "))
		os.Exit(1)
	}

	cs := newClientState(cfg, etcd)

	// start bootup server; this is an mDNS responder that this program looks for.
	// Peers that are also booting (not yet configured) will use this.
	// The lowest IP address will boot first.
	mdnsEtcdBootupServer(cfg, etcd)

	go cs.pollLoop()

	cs.stateTask()
	etcd.WriteFile()
}

