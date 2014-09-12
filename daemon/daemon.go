package local_peer_discovery_daemon

/*

Local peer discovery daemon

By Mark Sheahan, ScriptRock Inc

-- Does UDP broadcast to find local peers interested in a particular service.
-- Used for etcd auto-clustering where there is no global discovery service available, such as behind firewalls.

*/

import (
	"code.google.com/p/go-uuid/uuid"
	"encoding/json"
	"fmt"
	"github.com/ScriptRock/local_peer_discovery/common"
	"net"
	"net/http"
	"time"
)

var debug bool = false

func makeUDPBroadcastAddr(ipnet *net.IPNet) net.IP {
	bcip := ipnet.IP
	for b := 0; b < len(bcip); b++ {
		bcip[b] |= (ipnet.Mask[b] ^ 0xff)
	}
	return bcip
}

func shouldPollAddress(iface *net.Interface, ip *net.IP, ipnet *net.IPNet, port int) bool {
	_, masksize := ipnet.Mask.Size()
	// ipv4 only
	if masksize != 32 {
		return false
	}
	return true
}

func pollAddress(iface *net.Interface, ip *net.IP, ipnet *net.IPNet, port int, currentState []byte) {
	bcip := makeUDPBroadcastAddr(ipnet)
	socket, err := net.DialUDP("udp4", nil, &net.UDPAddr{
		IP:   bcip,
		Port: port,
	})
	if err != nil {
		fmt.Printf("Cannot open socket, interface %s addr %s:%d: %s\n",
			iface.Name, bcip.String(), port, err.Error())
	} else {
		//fmt.Printf("Tx  '%s'\n", string(currentState))
		expectedLen := len(currentState)
		i, err := socket.Write(currentState)
		if err != nil {
			fmt.Printf("Cannot write to socket, interface %s addr %s:%d: %s\n",
				iface.Name, bcip.String(), port, err.Error())
		} else {
			if i != expectedLen {
				fmt.Printf("Wrote only %d bytes (not %d) to socket...\n", i, expectedLen)
			}
		}
	}
}

func processRx(data []byte, bytesRead int, remoteAddr net.Addr, stateChans *StateUpdateChannels) {
	pd := PeerPacket{
		addr:   remoteAddr,
		packet: data[0:bytesRead],
	}
	stateChans.peerPacket <- pd
}

func openUDPListener(port int, stateChans *StateUpdateChannels) {
	if pc, err := net.ListenPacket("udp", fmt.Sprintf(":%d", port)); err != nil {
		stateChans.errors <- err
	} else {
		for {
			data := make([]byte, 4096)
			if bytesRead, remoteAddr, err := pc.ReadFrom(data); err != nil {
				fmt.Printf("Error from ReadFrom: %s\n", err.Error())
				stateChans.errors <- err
			} else {
				processRx(data, bytesRead, remoteAddr, stateChans)
			}
		}
	}
}

func openHTTPServer(port int, stateChans *StateUpdateChannels) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		s := stateChans.GetCurrentStateAll()
		slen := len(s)
		bytesToWrite := slen
		bytesWritten := 0
		for bytesWritten < bytesToWrite {
			b, err := w.Write(s[bytesWritten:])
			if err != nil {
				break
			}
			bytesWritten += b
		}
	}

	http.HandleFunc("/", handler)
	stateChans.errors <- http.ListenAndServe(fmt.Sprintf("127.0.0.1:%d", port), nil)
}

type PeerPacket struct {
	addr   net.Addr
	packet []byte
}

type LocalAddr struct {
	iface *net.Interface
	ip    *net.IP
	ipnet *net.IPNet
}

type LocalAddrStore struct {
	addr     *LocalAddr
	lastSeen time.Time
}

type StateUpdateChannels struct {
	myAddrs                  chan []*LocalAddr
	peerPacket               chan PeerPacket
	currentStateJsonRequest  chan int
	currentStateJsonResponse chan []byte
	currentStateAllRequest   chan int
	currentStateAllResponse  chan []byte
	errors                   chan error
}

type PeerState struct {
	meta        string
	group       string
	expireDelta time.Duration
	myselfUUID  string
	myAddrs     map[string]LocalAddrStore
	peers       map[string]Peer
}

func NewStateUpdateChannels() *StateUpdateChannels {
	s := new(StateUpdateChannels)
	s.myAddrs = make(chan []*LocalAddr)
	s.peerPacket = make(chan PeerPacket)
	s.currentStateJsonRequest = make(chan int)
	s.currentStateJsonResponse = make(chan []byte)
	s.currentStateAllRequest = make(chan int)
	s.currentStateAllResponse = make(chan []byte)
	s.errors = make(chan error)
	return s
}

func (s *StateUpdateChannels) GetCurrentStateJson(lim int) []byte {
	s.currentStateJsonRequest <- lim
	return <-s.currentStateJsonResponse
}

func (s *StateUpdateChannels) GetCurrentStateAll() []byte {
	s.currentStateAllRequest <- 0
	return <-s.currentStateAllResponse
}

type Peer struct {
	ip           net.IP
	lastSeen     time.Time
	seenDirectly bool
	seenMe       bool
	uuid         string
	meta         string
	localAddr    *LocalAddr
}

func (s *PeerState) cleanup() {
	// flush out exclude list, and old peers
	now := time.Now()
	for k, v := range s.myAddrs {
		if v.lastSeen.Add(s.expireDelta).Before(now) {
			if debug {
				fmt.Printf("Cleanup: timeout: deleting %s from exclude list\n", k)
			}
			delete(s.myAddrs, k)
		}
	}
	for k, v := range s.peers {
		if v.lastSeen.Add(s.expireDelta).Before(now) {
			if debug {
				fmt.Printf("Cleanup: timeout: deleting %s from peers list\n", k)
			}
			delete(s.peers, k)
		}
	}
}

func (s *PeerState) dumpPeers() {
	if true {
		fmt.Printf("%d peers:\n", len(s.peers))
		for _, v := range s.peers {
			fmt.Println(v)
		}
	}
}

func (s *PeerState) updateMyAddrs(myAddrs []*LocalAddr) {
	// update the local exclude list
	now := time.Now()
	for _, addr := range myAddrs {
		k := addr.ip.String()
		if _, ok := s.myAddrs[k]; !ok {
			if debug {
				fmt.Printf("Adding %s to exclude list\n", k)
			}
		}
		s.myAddrs[k] = LocalAddrStore{
			addr:     addr,
			lastSeen: now,
		}
	}
}

func (s *PeerState) runExcludes() {
	for k, _ := range s.myAddrs {
		if _, ok := s.peers[k]; ok {
			if debug {
				fmt.Printf("runExcludes: deleting %s from peers list\n", k)
			}
			delete(s.peers, k)
		}
	}
}

func (s *PeerState) localAddr(str string) *LocalAddr {
	for _, v := range s.myAddrs {
		if v.addr.ipnet.Contains(net.ParseIP(str)) {
			return v.addr
		}
	}
	return nil
}

func (s *PeerState) handlePeerPacket(peerPacket PeerPacket) {
	// yay, a peer
	peerAddr := peerPacket.addr.String()
	if peerHost, _, err := net.SplitHostPort(peerAddr); err != nil {
		fmt.Printf("Error parsing peerPacket address '%s': %s\n", peerAddr, err.Error())
	} else {
		if debug {
			fmt.Printf("Saw from '%s': %s\n", peerHost, string(peerPacket.packet))
		}
		ip := net.ParseIP(peerHost)
		if ip != nil {
			peer := Peer{
				ip:           ip,
				lastSeen:     time.Now(),
				seenDirectly: true,
				seenMe:       false,
				uuid:         "",
				meta:         "",
				localAddr:    s.localAddr(peerHost),
			}
			groupMatch := false
			// parse the incoming packet. Should be a JSON blob.
			var mif interface{}
			if err := json.Unmarshal(peerPacket.packet, &mif); err != nil {
				fmt.Printf("Invalid JSON from peer: %s\n", peerAddr, err.Error())
			} else {
				// valid JSON. Should be an associative array
				if m, ok := mif.(map[string]interface{}); ok {
					for k, vif := range m {
						vstr, is_string := vif.(string)
						_, is_map := vif.(map[string]interface{})

						if is_string && k == "uuid" {
							peer.uuid = vstr
						}
						if k == "meta" {
							peer.meta = vstr
						}
						if is_string && k == "group" && vstr == s.group {
							groupMatch = true
						}
						// parse the peer's list of peers; if we are in it, take note
						if is_map {
							la := s.localAddr(k)
							peer.seenMe = la != nil
						}
					}
				}
			}
			if groupMatch && peer.uuid != "" && peer.uuid != s.myselfUUID && peer.localAddr != nil {
				s.peers[peerHost] = peer
			}
		}
	}
}

func (s *PeerState) toJson(lim int) []byte {
	// produce response, send
	txState := make(map[string]interface{})
	txState["uuid"] = s.myselfUUID
	txState["meta"] = s.meta
	txState["group"] = s.group
	for k, v := range s.peers {
		peer := make(map[string]interface{})
		peer["seenDirectly"] = v.seenDirectly
		peer["seenMe"] = v.seenMe
		peer["uuid"] = v.uuid
		peer["meta"] = v.meta
		txState[k] = peer
	}
	if txStateJson, err := json.Marshal(&txState); err != nil {
		return []byte("{}")
	} else {
		return txStateJson
	}
}

func boolStr(b bool) string {
	if b {
		return "true"
	} else {
		return "false"
	}
}

func (s *PeerState) getAll() []byte {
	all := make([]common.PeerReport, 0)
	for k, v := range s.peers {
		peerReport := common.PeerReport{
			LocalAddr:    v.localAddr.ip.String(),
			PeerAddr:     k,
			PeerUUID:     v.uuid,
			PeerMeta:     v.meta,
			SeenDirectly: v.seenDirectly,
			PeerSeenMe:   v.seenMe,
		}
		all = append(all, peerReport)
	}
	if stateJson, err := json.Marshal(&all); err != nil {
		return []byte("[]")
	} else {
		return stateJson
	}
}

func stateLoop(meta string, group string, stateChans *StateUpdateChannels) {
	state := PeerState{
		meta:        meta,
		group:       group,
		expireDelta: 30 * time.Second,
		myselfUUID:  uuid.New(),
		myAddrs:     make(map[string]LocalAddrStore), // timeouts to update
		peers:       make(map[string]Peer),           // peer list
	}
	cleanup := time.Tick(5 * time.Second) // regular cleanups of things that are too old
	//fmt.Printf("UUID for myself: %s\n", state.myselfUUID)
	for {
		select {
		case myAddrs := <-stateChans.myAddrs:
			state.updateMyAddrs(myAddrs)
			state.runExcludes()
		case peerPacket := <-stateChans.peerPacket:
			state.handlePeerPacket(peerPacket)
			state.runExcludes()
		case lim := <-stateChans.currentStateJsonRequest:
			stateChans.currentStateJsonResponse <- state.toJson(lim)
		case <-stateChans.currentStateAllRequest:
			stateChans.currentStateAllResponse <- state.getAll()
		case <-cleanup:
			state.cleanup()
			state.runExcludes()
			//state.dumpPeers()
		}
	}
}

func pollOnAllInterfaces(port int, stateChans *StateUpdateChannels) {
	myAddrs := make([]*LocalAddr, 0)

	currentState := stateChans.GetCurrentStateJson(512)

	if ifaces, err := net.Interfaces(); err != nil {
		fmt.Println(err)
	} else {
		for _, iface := range ifaces {
			//fmt.Println(iface)
			if (iface.Flags & net.FlagBroadcast) != 0 {
				if iface_addrs, err := iface.Addrs(); err != nil {
					fmt.Printf("Error getting addresses for interface '%s': %s\n", iface.Name, err.Error())
				} else {
					for _, iface_addr := range iface_addrs {
						//fmt.Println(iface_addr)
						ipstr := iface_addr.String()
						if ip, ipnet, err := net.ParseCIDR(ipstr); err != nil {
							fmt.Printf("Error parsing IP address '%s'\n", ipstr, err.Error())
						} else {
							//fmt.Println(ip)
							//fmt.Println(ipnet)
							if shouldPollAddress(&iface, &ip, ipnet, port) {
								localAddr := LocalAddr{
									iface: &iface,
									ip:    &ip,
									ipnet: ipnet,
								}
								myAddrs = append(myAddrs, &localAddr)
								pollAddress(&iface, &ip, ipnet, port, currentState)
							}
						}
					}
				}
			} else {
				//fmt.Printf("Interface '%s' doesn't support broadcast\n", iface.Name)
			}
		}
	}

	stateChans.myAddrs <- myAddrs
}

func main() {

	cfg, err := common.New()
	if err != nil {
		fmt.Printf("Error parsing options: %s\n", err.Error())
		return
	}

	debug = cfg.Debug
	finished := false

	stateChans := NewStateUpdateChannels()

	go stateLoop(cfg.Meta, cfg.Group, stateChans)
	go openUDPListener(cfg.UDPPort, stateChans)
	go openHTTPServer(cfg.QueryPort, stateChans)
	go func() {
		err := <-stateChans.errors
		finished = true
		panic(fmt.Sprintf("Error: '%s'\n", err.Error()))
	}()

	loops := 0
	for !finished {
		pollOnAllInterfaces(cfg.UDPPort, stateChans)
		loops = loops + 1
		if cfg.MaxLoops > 0 && loops >= cfg.MaxLoops {
			break
		}
		time.Sleep(cfg.PollInterval)
	}
}
