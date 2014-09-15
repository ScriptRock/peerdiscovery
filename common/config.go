package common

import (
	"code.google.com/p/go-uuid/uuid"
	"fmt"
	go_flags "github.com/jessevdk/go-flags"
	"io/ioutil"
	"strings"
	"time"
)

type Config struct {
	UUID               string `long:"uuid" description:"UUID used for mDNS hostname and service instance"`
	MDNSInstance       string `long:"mdns_instance" description:"mDNS instance name (default is uuid)"`
	MDNSBootupService  string `long:"mdns_bootup_service" description:"mDNS service name for discovering still-polling peers (default '_scriptrock_etcd_searching._tcp')"`
	MDNSService        string `long:"mdns_service" description:"mDNS service name (default '_scriptrock_etcd._tcp')"`
	MDNSDomain         string `long:"mdns_domain" description:"mDNS domain (default 'local')"`
	PollInterval       time.Duration
	PollIntervalSetter func(int) `long:"poll_interval" description:"polling interval when trying to find peers (default 1s)"`
	MaxLoops           int       `long:"max_loops" description:"maximum number of loops to poll before writing etcd conf (default 10)"`
	Debug              bool      `long:"debug" description:"Debug mode"`
}

func (c *Config) load(argsin []string) ([]string, error) {
	clusterUUIDPath := "/etc/etcd/cluster_uuid"
	if clusterUUID, err := ioutil.ReadFile(clusterUUIDPath); err != nil {
		c.UUID = uuid.New()
		if err := ioutil.WriteFile(clusterUUIDPath, []byte(c.UUID), 0644); err != nil {
			fmt.Printf("Could not open '%s' for writing: %s\n", clusterUUIDPath, err.Error())
		}
	} else {
		u := strings.TrimSpace(string(clusterUUID))
		if uuid.Parse(u) == nil {
			fmt.Printf("UUID from file '%s' is invalid\n", clusterUUIDPath)
			u = uuid.New()
		}
		c.UUID = u
	}

	// TODO FIXME load UUID from a file
	c.MDNSInstance = c.UUID
	c.MDNSBootupService = "_scriptrock_etcd_bootup._tcp"
	c.MDNSService = "_scriptrock_etcd._tcp"
	c.MDNSDomain = "local"
	c.PollInterval = 1 * time.Second
	c.MaxLoops = 10
	c.Debug = false

	c.PollIntervalSetter = func(i int) {
		c.PollInterval = time.Duration(i) * time.Second
	}
	return go_flags.NewParser(c, go_flags.IgnoreUnknown).ParseArgs(argsin)
}

func NewConfig(argsin []string) (*Config, []string, error) {
	c := new(Config)
	argsout, err := c.load(argsin)
	return c, argsout, err
}
