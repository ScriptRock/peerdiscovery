package common

import (
	go_flags "github.com/jessevdk/go-flags"
	"os"
	"strconv"
	"time"
)

type ConfigurationOptions struct {
	// Example of a required flag
	MetaSetter         func(string) `long:"meta" description:"Metadata to include with this node"`
	GroupSetter        func(string) `long:"group" description:"Group UUID"`
	UDPPortSetter      func(int)    `long:"udp_port" description:"Port to listen on & send to for UDP broadcast"`
	QueryPortSetter    func(int)    `long:"query_port" description:"Port for local http queries"`
	PollIntervalSetter func(int)    `long:"poll_interval" description:"Period (seconds) between UDP broadcasts"`
	MaxLoopsSetter     func(int)    `long:"max_loops" description:"Number of times we may query or idle-poll before terminating"`
	DebugSetter        func()       `long:"debug" description:"Debug mode"`
	MatchedOnlySetter  func(bool)   `long:"matched_only" description:"Only show peers whom have seen me (default true)"`
	EtcdConfSetter     func(string) `long:"etcd_conf" description:"Output is in etcd conf file format, to specified file"`
}

type Config struct {
	Meta         string
	Group        string
	UDPPort      int
	QueryPort    int
	PollInterval time.Duration
	MaxLoops     int
	Debug        bool
	MatchedOnly  bool
	EtcdConf     bool
	EtcdConfPath string
	Argv         []string
}

type PeerReport struct {
	LocalAddr    string
	PeerAddr     string
	PeerUUID     string
	PeerMeta     string
	SeenDirectly bool
	PeerSeenMe   bool
}

func (c *Config) load(is_server bool) error {
	default_port := 44001
	default_udp_port := default_port
	default_query_port := default_port

	c.Meta = ""
	c.Group = "default"
	c.UDPPort = default_udp_port
	c.QueryPort = default_query_port
	if is_server {
		c.PollInterval = 5 * time.Second
		c.MaxLoops = 0
	} else {
		c.PollInterval = 1 * time.Second
		c.MaxLoops = 10
	}
	c.Debug = false
	c.MatchedOnly = true
	c.EtcdConf = false
	c.EtcdConfPath = ""

	if s := os.Getenv("SCRIPTROCK_LOCAL_PEER_DISCOVERY_META"); s != "" {
		c.Meta = s
	}
	if s := os.Getenv("SCRIPTROCK_LOCAL_PEER_DISCOVERY_GROUP"); s != "" {
		c.Group = s
	}
	if s := os.Getenv("SCRIPTROCK_LOCAL_PEER_DISCOVERY_UDP_PORT"); s != "" {
		if i, err := strconv.Atoi(s); err == nil {
			c.UDPPort = i
		}
	}
	if s := os.Getenv("SCRIPTROCK_LOCAL_PEER_DISCOVERY_QUERY_PORT"); s != "" {
		if i, err := strconv.Atoi(s); err == nil {
			c.QueryPort = i
		}
	}
	if s := os.Getenv("SCRIPTROCK_LOCAL_PEER_DISCOVERY_POLL_INTERVAL"); s != "" {
		if i, err := strconv.Atoi(s); err == nil && i > 1 {
			c.PollInterval = time.Duration(i) * time.Second
		}
	}
	if s := os.Getenv("SCRIPTROCK_LOCAL_PEER_DISCOVERY_DEBUG"); s != "" {
		c.Debug = true
	}
	if s := os.Getenv("SCRIPTROCK_LOCAL_PEER_DISCOVERY_MATCHED_ONLY"); s != "" {
		if s == "true" {
			c.MatchedOnly = true
		}
		if s == "false" {
			c.MatchedOnly = false
		}
	}

	opts := new(ConfigurationOptions)
	opts.MetaSetter = func(s string) {
		c.Meta = s
	}
	opts.GroupSetter = func(s string) {
		c.Group = s
	}
	opts.UDPPortSetter = func(p int) {
		c.UDPPort = p
	}
	opts.QueryPortSetter = func(p int) {
		c.QueryPort = p
	}
	opts.PollIntervalSetter = func(p int) {
		if p >= 1 {
			c.PollInterval = time.Duration(p) * time.Second
		}
	}
	opts.MaxLoopsSetter = func(p int) {
		c.MaxLoops = p
	}
	opts.DebugSetter = func() {
		c.Debug = true
	}
	opts.MatchedOnlySetter = func(b bool) {
		c.MatchedOnly = true
	}
	opts.EtcdConfSetter = func(s string) {
		c.EtcdConf = true
		c.EtcdConfPath = s
	}

	argv, err := go_flags.Parse(opts)

	c.Argv = argv

	return err
}

func New(is_server bool) (*Config, error) {
	c := new(Config)
	err := c.load(is_server)
	return c, err
}
