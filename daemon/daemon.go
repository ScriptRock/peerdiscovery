package daemon

/*

Local peer discovery daemon

By Mark Sheahan, ScriptRock Inc

-- Used for etcd auto-clustering where there is no global discovery service available, such as behind firewalls.
-- All this does is set up an mDNS responder, which is basically 'yes, i'm running etcd, connect to me'

*/

import (
	"fmt"
	"github.com/ScriptRock/mdns"
	"github.com/ScriptRock/peerdiscovery/common"
	"os"
	"strings"
	"time"
)

var debug bool = false

func mdnsEtcdServer(cfg *common.Config, etcd *common.EtcdConfig) {
	service := &mdns.MDNSService{
		Instance: cfg.MDNSInstance,
		Service:  cfg.MDNSService,
		Domain:   cfg.MDNSDomain,
		Port:     etcd.PeerPort,
	}
	if err := service.Init(); err != nil {
		fmt.Println("mDNS error", err)
	}

	mdns.NewServer(&mdns.Config{Zone: service})
}

func mdnsHostServer(cfg *common.Config) {
	service := &mdns.MDNSService{
		AliasHostName: cfg.UUID,
		Domain:        cfg.MDNSDomain,
	}
	if err := service.Init(); err != nil {
		fmt.Println("mDNS error", err)
	}

	mdns.NewServer(&mdns.Config{Zone: service})
}

func Daemon() {
	cfg, etcd, args := common.LoadConfigs()
	if len(args) > 1 {
		fmt.Printf("Error parsing options; un-parsed options remain: %s\n", strings.Join(args[1:], ", "))
		os.Exit(1)
	}
	debug = cfg.Debug

	go mdnsHostServer(cfg)
	go mdnsEtcdServer(cfg, etcd)

	// infinite loop; other goroutines handle everything
	for {
		time.Sleep(time.Hour)
	}
}

