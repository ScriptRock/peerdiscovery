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
	"github.com/ScriptRock/peerdiscovery/common"
	"io/ioutil"
	"net/http"
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

func main() {
	cfg, err := common.New()
	if err != nil {
		fmt.Printf("Error parsing options: %s\n", err.Error())
		return
	}

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
