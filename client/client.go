package main

/*

Local peer discovery client

By Mark Sheahan, ScriptRock Inc

-- Polls local daemon to obtain the current peers list
-- Does UDP broadcast to find local peers interested in a particular service.
-- Used for etcd auto-clustering where there is no global discovery service available, such as behind firewalls.

*/

import (
	"encoding/json"
	"fmt"
	"github.com/ScriptRock/mdns"
	"github.com/ScriptRock/peerdiscovery/common"
	"io/ioutil"
	"net/http"
	"sort"
	"time"
)

func boolStr(b bool) string {
	if b {
		return "true"
	} else {
		return "false"
	}
}

func pollServer(cfg *common.Config) ([]common.PeerReport, error) {
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

func makeEtcdConf(cfg *common.Config) {
	loops := 0
	for {
		all, err := pollServer(cfg)
		if err != nil {
			fmt.Printf("Error polling server: %s\n", err.Error())
		}

		addressesLowerThanMine := make([]string, 0)
		for _, p := range all {
			if !cfg.MatchedOnly || (cfg.MatchedOnly && p.SeenDirectly && p.PeerSeenMe) {
				if p.PeerAddr < p.LocalAddr {
					addressesLowerThanMine = append(addressesLowerThanMine, p.PeerAddr)
				}
			}
		}
		sort.Strings(addressesLowerThanMine)

		peerStr := "["
		for i, a := range addressesLowerThanMine {
			if i > 0 {
				peerStr = peerStr + ","
			}
			peerStr = peerStr + fmt.Sprintf("\"%s:%d\"", a, 7001)
		}
		peerStr = peerStr + "]"
		fmt.Println("Loop", loops, " peers ", peerStr)

		loops = loops + 1
		if cfg.MaxLoops > 0 && loops >= cfg.MaxLoops {
			break
		}
		time.Sleep(cfg.PollInterval)
	}
}

func mDnsClient() {
	// Make a channel for results and start listening
	entriesCh := make(chan *mdns.ServiceEntry, 4)
	go func() {
		for entry := range entriesCh {
			fmt.Printf("Got new entry: %v\n", entry)
		}
	}()

	// Start the lookup
	mdns.Lookup("_etcd._tcp", entriesCh)
	close(entriesCh)
	time.Sleep(time.Hour)
}

func client() {
	cfg, err := common.New(false)
	if err != nil {
		fmt.Printf("Error parsing options: %s\n", err.Error())
		return
	}

	if cfg.EtcdConf {
		makeEtcdConf(cfg)
	} else {
		if all, err := pollServer(cfg); err != nil {
			fmt.Printf("Error polling server: %s\n", err.Error())
		} else {
			for _, p := range all {
				if !cfg.MatchedOnly || (cfg.MatchedOnly && p.SeenDirectly && p.PeerSeenMe) {
					fmt.Printf("%s %s %s %s %s %s\n",
						p.LocalAddr,
						p.PeerAddr,
						boolStr(p.SeenDirectly),
						boolStr(p.PeerSeenMe),
						p.PeerUUID,
						p.PeerMeta)
				}
			}
		}
	}
}

func main() {
	mDnsClient()
	//client()
}
