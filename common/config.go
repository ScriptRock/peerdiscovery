package common

import (
	"code.google.com/p/go-uuid/uuid"
	"fmt"
	go_flags "github.com/jessevdk/go-flags"
	"io/ioutil"
	"strings"
	"time"
	"os"
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

var ClusterInstanceUUIDPath string = "/etc/etcd/cluster_instance_uuid"

func PanicDuplicateClusterInstanceUUID() {
	fmt.Printf("Duplicate cluster instance UUID encountered; deleting old UUID and exiting\n")
	if err := os.Remove(ClusterInstanceUUIDPath); err != nil {
		fmt.Printf("Could not delete '%s': %s\n", ClusterInstanceUUIDPath, err.Error())
	}
	os.Exit(1)
}

func LoadClusterInstanceUUID() string {
	clusterUUID := ""
	// attempt to 
	if fileData, err := ioutil.ReadFile(ClusterInstanceUUIDPath); err != nil {
		clusterUUID = uuid.New()
		if err := ioutil.WriteFile(ClusterInstanceUUIDPath, []byte(clusterUUID), 0644); err != nil {
			fmt.Printf("Could not open '%s' for writing: %s\n", ClusterInstanceUUIDPath, err.Error())
		}
	} else {
		clusterUUID = strings.TrimSpace(string(fileData))
		if uuid.Parse(clusterUUID) == nil {
			fmt.Printf("UUID from file '%s' is invalid\n", ClusterInstanceUUIDPath)
			clusterUUID = uuid.New()
			// write out a new valid one
			if err := ioutil.WriteFile(ClusterInstanceUUIDPath, []byte(clusterUUID), 0644); err != nil {
				fmt.Printf("Could not open '%s' for writing: %s\n", ClusterInstanceUUIDPath, err.Error())
			}
		}
	}
	fmt.Printf("Cluster Instance UUID: %s\n", clusterUUID)
	return clusterUUID
}

func (c *Config) load(argsin []string) ([]string, error) {
	c.UUID = LoadClusterInstanceUUID()
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
