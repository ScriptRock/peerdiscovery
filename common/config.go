package common

import (
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"time"

	go_flags "github.com/jessevdk/go-flags"
	"github.com/pborman/uuid"
)

type Config struct {
	UUID               string `long:"uuid" description:"UUID used for mDNS hostname and service instance"`
	MDNSInstance       string `long:"mdns_instance" description:"mDNS instance name (default is uuid)"`
	MDNSService        string `long:"mdns_service" description:"mDNS service name (default '_scriptrock_etcd._tcp')"`
	MDNSDomain         string `long:"mdns_domain" description:"mDNS domain (default 'local')"`
	PollInterval       time.Duration
	PollIntervalSetter func(int) `long:"poll_interval" description:"polling interval when trying to find peers (default 1s)"`
	MaxLoops           int       `long:"max_loops" description:"maximum number of loops to poll before writing etcd conf (default 10)"`
	AvahiConfPath      string    `long:"avahi_conf_path" description:"where to write avahi service definition to (default /etc/avahi/services/etcd.service)"`
	Debug              bool      `long:"debug" description:"Debug mode"`
}

var ClusterInstanceUUIDPath string = "/etc/machine-id"

func DuplicateClusterInstanceUUID() {
	fmt.Printf("Duplicate cluster instance UUID encountered; must delete: '%s' and reboot\n", ClusterInstanceUUIDPath)
	if err := os.Remove(ClusterInstanceUUIDPath); err != nil {
		fmt.Printf("Could not delete '%s': %s\n", ClusterInstanceUUIDPath, err.Error())
	}
}

func uuidValid(u string) bool {
	if len(u) == 32 {
		u = u[0:8] + "-" + u[8:12] + "-" + u[12:16] + "-" + u[16:20] + "-" + u[20:]
	}
	if uuid.Parse(u) != nil {
		return true
	}
	return false
}

func LoadClusterInstanceUUID() string {
	clusterUUID := ""
	if fileData, err := ioutil.ReadFile(ClusterInstanceUUIDPath); err != nil {
		clusterUUID = uuid.New()
		if err := ioutil.WriteFile(ClusterInstanceUUIDPath, []byte(clusterUUID), 0644); err != nil {
			fmt.Printf("Could not open '%s' for writing: %s\n", ClusterInstanceUUIDPath, err.Error())
		}
	} else {
		clusterUUID = strings.TrimSpace(string(fileData))
		if !uuidValid(clusterUUID) {
			fmt.Printf("UUID from file '%s' is invalid\n", ClusterInstanceUUIDPath)
			clusterUUID = uuid.New()
			clusterUUID = strings.Replace(clusterUUID, "-", "", -1)
			// write out a new valid one
			if err := ioutil.WriteFile(ClusterInstanceUUIDPath, []byte(clusterUUID), 0644); err != nil {
				fmt.Printf("Could not open '%s' for writing: %s\n", ClusterInstanceUUIDPath, err.Error())
			}
		}
	}
	fmt.Printf("Cluster Instance UUID (machine-id): %s\n", clusterUUID)
	return clusterUUID
}

func (c *Config) load(argsin []string) ([]string, error) {
	c.UUID = LoadClusterInstanceUUID()
	c.MDNSInstance = c.UUID
	c.MDNSService = "_scriptrock_etcd._tcp"
	c.MDNSDomain = "local"
	c.PollInterval = 1 * time.Second
	c.MaxLoops = 10
	c.AvahiConfPath = "/etc/avahi/services/etcd.service"
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
