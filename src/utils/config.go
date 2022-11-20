package utils

// configRaft configuration for raft node
type ConfigRaft struct {
	NodeId    string `mapstructure:"node_id"`
	Address   string `mapstructure:"address"`
	VolumeDir string `mapstructure:"volume_dir"`
}

// configServer configuration for HTTP server
type ConfigServer struct {
	Address     string `mapstructure:"address"`
	JoinAddress string `mapstructure:"join_address"`
	IsServer    bool   `mapstructure:"is_server"`
}

// config configuration
type Config struct {
	Server ConfigServer `mapstructure:"server"`
	Raft   ConfigRaft   `mapstructure:"raft"`
	Peers  []string     `mapstructure:"peers"`
}
