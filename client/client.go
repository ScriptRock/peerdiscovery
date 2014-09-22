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
	"github.com/ScriptRock/peerdiscovery/common"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func boolStr(b bool) string {
	if b {
		return "true"
	} else {
		return "false"
	}
}

type AvahiBrowseResult struct {
	//	=;eth1;IPv4;test;_scriptrock_etcd._tcp;local;mark-ubuntu-vm.local;192.168.56.101;12346;
	Type          string // resolved =, added +, removed -. We filter down to resolved only
	InterfaceName string // eth1
	Protocol      string // IPv4
	Name          string // test
	Service       string // _scriptrock_etcd._tcp
	Domain        string // local
	Host          string // mark-ubuntu-vm.local
	IPString      string // 192.168.56.101
	IPv4          net.IP
	IPv6          net.IP
	PortString    string // 12346
	Port          int
}

type ClientState struct {
	cfg                   *common.Config
	etcd                  *common.EtcdConfig
	mdnsPeerServerEntries chan *AvahiBrowseResult
	discoveryURL          chan string
	pollEvent             chan int
}

func newClientState(cfg *common.Config, etcd *common.EtcdConfig) *ClientState {
	return &ClientState{
		cfg:  cfg,
		etcd: etcd,
		mdnsPeerServerEntries: make(chan *AvahiBrowseResult),
		discoveryURL:          make(chan string, 2),
		pollEvent:             make(chan int),
	}
}

func parseAvahiBrowse(data []byte) []*AvahiBrowseResult {
	results := make([]*AvahiBrowseResult, 0)
	lines := regexp.MustCompile("\\r?\\n").Split(string(data), -1)
	for _, line := range lines {
		fields := regexp.MustCompile(";").Split(line, -1)
		if len(fields) >= 9 && fields[0] == "=" {
			a := &AvahiBrowseResult{
				Type:          fields[0],
				InterfaceName: fields[1],
				Protocol:      fields[2],
				Name:          fields[3],
				Service:       fields[4],
				Domain:        fields[5],
				Host:          fields[6],
				IPString:      fields[7],
				IPv4:          nil,
				IPv6:          nil,
				PortString:    fields[8],
				Port:          0,
			}
			if v, err := strconv.Atoi(a.PortString); err == nil {
				a.Port = v
			}
			if a.Protocol == "IPv4" {
				a.IPv4 = net.ParseIP(a.IPString)
			}
			if a.Protocol == "IPv6" {
				a.IPv6 = net.ParseIP(a.IPString)
			}
			results = append(results, a)
		}
	}
	return results
}

func avahiPrefix() string {
	wrapperPath := ""

	wrapperPath = "/opt/usr/bin/"
	if _, err := os.Stat(wrapperPath); err == nil {
		return wrapperPath
	}
	wrapperPath = "/opt/scriptrock_utils/docker_avahi/docker_avahi_ssh.sh"
	if _, err := os.Stat(wrapperPath); err == nil {
		return wrapperPath + " "
	}
	return ""
}

func runAvahiPublish(id string, service string, port int) *os.Process {
	avahiWrapper := avahiPrefix()

	// poll loop until the daemon is up
	for {
		command := strings.TrimSpace(fmt.Sprintf("%savahi-browse --terminate -a", avahiWrapper))
		commandParts := regexp.MustCompile("\\s+").Split(command, -1)
		if _, err := exec.Command(commandParts[0], commandParts[1:]...).Output(); err != nil {
			time.Sleep(time.Second)
		} else {
			break
		}
	}

	command := strings.TrimSpace(fmt.Sprintf("%savahi-publish-service %s %s %d", avahiWrapper, id, service, port))
	commandParts := regexp.MustCompile("\\s+").Split(command, -1)

	cmd := exec.Command(commandParts[0], commandParts[1:]...)
	cmd.SysProcAttr = &syscall.SysProcAttr{}
	//cmd.SysProcAttr.Pdeathsig = syscall.SIGTERM
	cmd.SysProcAttr.Setsid = false
	cmd.SysProcAttr.Setpgid = false
	// start process
	if err := cmd.Start(); err != nil {
		fmt.Printf("Error starting avahi command: '%s' error '%s'\n", command, err.Error())
		return nil
	}
	return cmd.Process
}

func runAvahiBrowse(service string, results chan *AvahiBrowseResult) {
	avahiWrapper := avahiPrefix()

	command := strings.TrimSpace(fmt.Sprintf("%savahi-browse --terminate --parsable --no-db-lookup --ignore-local --resolve %s", avahiWrapper, service))
	commandParts := regexp.MustCompile("\\s+").Split(command, -1)

	out, err := exec.Command(commandParts[0], commandParts[1:]...).Output()
	if err != nil {
		fmt.Printf("Error running avahi: command '%s' error '%s'\n", command, err.Error())
	} else {
		parsed := parseAvahiBrowse(out)
		for _, p := range parsed {
			results <- p
		}
	}
}

func (cs *ClientState) WriteAvahiServiceFile() {
	// wrap each peer in quotes
	conf := fmt.Sprintf(
		`<?xml version="1.0" standalone='no'?><!--*-nxml-*-->
<!DOCTYPE service-group SYSTEM "avahi-service.dtd">
<service-group>
  <name>%s</name>
  <service>
    <type>%s</type>
    <port>%d</port>
  </service>
</service-group>
`,
		cs.cfg.UUID,
		cs.cfg.MDNSService,
		cs.etcd.PeerPort)

	fmt.Printf("Writing avahi conf file to '%s'\n", cs.cfg.AvahiConfPath)
	if err := ioutil.WriteFile(cs.cfg.AvahiConfPath, []byte(conf), 0644); err != nil {
		fmt.Printf("Could not write conf file '%s': %s\n", cs.cfg.AvahiConfPath, err.Error())
	}
}

func (cs *ClientState) pollLoop() {
	for {
		runAvahiBrowse(cs.cfg.MDNSService, cs.mdnsPeerServerEntries)

		// run avahi browse to see nearby things
		cs.pollEvent <- 0
		time.Sleep(cs.cfg.PollInterval)
	}
}

func (cs *ClientState) checkEnt(ent *AvahiBrowseResult) (*net.Interface, net.IP, net.IP, error, error) {
	// only use IPv4
	peerIP := ent.IPv4
	if peerIP == nil {
		return nil, nil, nil, fmt.Errorf("No IPv4 address present"), nil
	}
	// This technically restricts valid configurations that go through routers. Will need to re-visit.
	// The objective is to prevent NATs where the return path will not work, not routers where the return path will.
	// Eventually, must set up a tcp/http server on the destination host that echoes back the connecting IP address,
	// and compare against that.
	// There is also an issue with multiple addresses on the same subnet on the same interface; but this is dumb anyway
	iface, _, myIP, err := common.LocalNetForIp(peerIP)
	if err == nil && myIP.Equal(peerIP) {
		return nil, nil, nil, fmt.Errorf("IP address is self (%s = %s)", myIP.String(), peerIP.String()), nil
	}
	if strings.HasPrefix(ent.Name, cs.cfg.UUID) {
		// This is bad; duplicate UUID from someone that isn't us. Presumably caused by a cloned VM.
		// In this case, panic, delete old id, die, and on the next respawn we'll regenerate the id
		common.DuplicateClusterInstanceUUID()
		fatalErr := fmt.Errorf("Prefix UUID is from self")
		return nil, nil, nil, fatalErr, fatalErr
	}
	return iface, myIP, peerIP, err, nil
}

func (cs *ClientState) peerMDNSHostname(ent *AvahiBrowseResult) string {
	return ent.Name
}

func (cs *ClientState) stateTask() (errOut error) {
	polls := 0
	lastPollWithHigherPeer := 0
	finished := false
	errOut = nil

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
			// peer etcd server. It may still be booting though.
			// do an HTTP request to the server to see if it truly exists
			if iface, localIP, peerIP, err, fatalErr := cs.checkEnt(ent); fatalErr != nil {
				finished = true
				fmt.Printf("Fatal error from peer server entry: %s\n", err.Error())
				errOut = fatalErr
			} else if err != nil {
				fmt.Println("etcd server", ent, "invalid", err)
			} else {
				peerMDNSHostname := cs.peerMDNSHostname(ent)
				peerPort := ent.Port
				fmt.Printf("etcd server mDNS response: IP %s mDNS hostname %s\n", peerIP.String(), peerMDNSHostname)
				url := fmt.Sprintf("http://%s:%d/v2/keys/", peerIP.String(), cs.etcd.ClientPort)
				if _, err := http.Get(url); err != nil {
					fmt.Printf("Peer at '%s' not available yet: %s\n", url, err.Error())
					cs.etcd.AddBootingPeer(iface, localIP, peerIP, peerPort)
					if localIP.String() < peerIP.String() {
						// we are lower; keep going
					} else {
						lastPollWithHigherPeer = polls
					}
				} else {
					fmt.Printf("Peer etcd server found on %s (%s); exiting\n", url, peerIP)
					cs.etcd.AddServerPeer(iface, localIP, peerIP, peerPort)
					cs.etcd.DiscoveryURL = ""
					finished = true
				}
			}
		case url := <-cs.discoveryURL:
			// url is already validated
			cs.etcd.DiscoveryURL = url
			finished = true
		}
	}

	return errOut
}

func (cs *ClientState) validateDiscoveryURL(url string) bool {
	if url != "" {
		if _, err := http.Get(url); err != nil {
			fmt.Printf("Poll discovery URL '%s' returns error: %s\n", url, err.Error())
		} else {
			cs.discoveryURL <- url
			return true
		}
	}
	return false
}

func (cs *ClientState) checkDiscoveryURL() bool {
	if cs.validateDiscoveryURL(cs.etcd.DiscoveryURL) {
		return true
	} else if cs.validateDiscoveryURL(os.Getenv("ETCD_DISCOVERY")) {
		return true
	} else {
		// check file
		urlFile := "/etc/etcd/discovery_url"
		if fileData, err := ioutil.ReadFile(urlFile); err != nil {
			// no file; ignore
		} else {
			if cs.validateDiscoveryURL(strings.TrimSpace(string(fileData))) {
				return true
			}
		}
	}
	return false
}

func Client() {
	cfg, etcd, fleet, args, err := common.LoadConfigs()
	if err != nil || len(args) > 1 {
		fmt.Printf("Error parsing options; un-parsed options remain: %s\n", strings.Join(args[1:], ", "))
	} else {
		cs := newClientState(cfg, etcd)

		cs.WriteAvahiServiceFile()

		// if a discovery URL is present, test it and publish if successful
		usingDiscoveryURL := cs.checkDiscoveryURL()

		// otherwise start mDNS polling
		if !usingDiscoveryURL {
			go cs.pollLoop()
		}

		err := cs.stateTask()
		if err == nil {
			etcd.WriteFile()
			fleet.WriteFile(etcd)
		} else {
			os.Exit(1)
		}
	}
}
