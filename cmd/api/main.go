package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"
	"ysf/raftsample/fsm"
	"ysf/raftsample/server"
	"ysf/raftsample/server/raft_handler"

	"github.com/dgraph-io/badger/v2"
	"github.com/hashicorp/raft"
	raftboltdb "github.com/hashicorp/raft-boltdb"
	"github.com/spf13/viper"
)

// configRaft configuration for raft node
type configRaft struct {
	NodeId    string `mapstructure:"node_id"`
	Address   string `mapstructure:"address"`
	VolumeDir string `mapstructure:"volume_dir"`
}

// configServer configuration for HTTP server
type configServer struct {
	Address     string `mapstructure:"address"`
	JoinAddress string `mapstructure:"join_address"`
}

// config configuration
type config struct {
	Server configServer `mapstructure:"server"`
	Raft   configRaft   `mapstructure:"raft"`
}

const (
	serverAddress     = "SERVER_ADDRESS"
	serverJoinAddress = "SERVER_JOIN_ADDRESS"

	raftNodeId  = "RAFT_NODE_ID"
	raftAddress = "RAFT_ADDRESS"
	raftVolDir  = "RAFT_VOL_DIR"
)

var confKeys = []string{
	serverAddress,

	raftNodeId,
	raftAddress,
	raftVolDir,
}

const (
	// The maxPool controls how many connections we will pool.
	maxPool = 3

	// The timeout is used to apply I/O deadlines. For InstallSnapshot, we multiply
	// the timeout by (SnapshotSize / TimeoutScale).
	// https://github.com/hashicorp/raft/blob/v1.1.2/net_transport.go#L177-L181
	tcpTimeout = 10 * time.Second

	// The `retain` parameter controls how many
	// snapshots are retained. Must be at least 1.
	raftSnapShotRetain = 2

	// raftLogCacheSize is the maximum number of logs to cache in-memory.
	// This is used to reduce disk I/O for the recently committed entries.
	raftLogCacheSize = 512
)

func retryJoin(conf config) int {
	form := raft_handler.RequestJoin{
		NodeID:      conf.Raft.NodeId,
		RaftAddress: conf.Raft.Address,
	}

	postBody, _ := json.Marshal(form)
	responseBody := bytes.NewBuffer(postBody)

	fmt.Println(conf.Server.JoinAddress)
	fmt.Printf("%+v", form)
	fmt.Println(postBody)

	resp, err := http.Post(fmt.Sprintf("%s/raft/join", conf.Server.JoinAddress), "application/json", responseBody)

	//Handle Error
	if err != nil {
		log.Fatal(err)
	} else {
		log.Println(resp)
	}
	defer resp.Body.Close()

	return resp.StatusCode
}

// main entry point of application start
// run using CONFIG=config.yaml ./program
func main() {

	var v = viper.New()
	v.AutomaticEnv()
	if err := v.BindEnv(confKeys...); err != nil {
		log.Fatal(err)
		return
	}

	conf := config{
		Server: configServer{
			Address:     v.GetString(serverAddress),
			JoinAddress: v.GetString(serverJoinAddress),
		},
		Raft: configRaft{
			NodeId:    v.GetString(raftNodeId),
			Address:   v.GetString(raftAddress),
			VolumeDir: v.GetString(raftVolDir),
		},
	}

	log.Printf("%+v\n", conf)

	// Preparing badgerDB
	badgerOpt := badger.DefaultOptions(conf.Raft.VolumeDir)
	badgerDB, err := badger.Open(badgerOpt)
	if err != nil {
		log.Fatal(err)
		return
	}

	defer func() {
		if err := badgerDB.Close(); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "error close badgerDB: %s\n", err.Error())
		}
	}()

	raftConf := raft.DefaultConfig()
	raftConf.LocalID = raft.ServerID(conf.Raft.NodeId)
	raftConf.SnapshotThreshold = 1024

	fsmStore := fsm.NewBadger(badgerDB)

	store, err := raftboltdb.NewBoltStore(filepath.Join(conf.Raft.VolumeDir, "raft.dataRepo"))
	if err != nil {
		log.Fatal(err)
		return
	}

	// Wrap the store in a LogCache to improve performance.
	cacheStore, err := raft.NewLogCache(raftLogCacheSize, store)
	if err != nil {
		log.Fatal(err)
		return
	}

	snapshotStore, err := raft.NewFileSnapshotStore(conf.Raft.VolumeDir, raftSnapShotRetain, os.Stdout)
	if err != nil {
		log.Fatal(err)
		return
	}

	tcpAddr, err := net.ResolveTCPAddr("tcp", conf.Raft.Address)
	if err != nil {
		log.Fatal(err)
		return
	}

	transport, err := raft.NewTCPTransport(conf.Raft.Address, tcpAddr, maxPool, tcpTimeout, os.Stdout)
	if err != nil {
		log.Fatal(err)
		return
	}

	raftServer, err := raft.NewRaft(raftConf, fsmStore, cacheStore, store, snapshotStore, transport)
	if err != nil {
		log.Fatal(err)
		return
	}

	if len(conf.Server.JoinAddress) > 0 {

		for {
			statusCode := retryJoin(conf)
			time.Sleep(4 * time.Second)
			if statusCode == 200 {
				break
			}
		}
	} else {

		// always start single server as a leader
		configuration := raft.Configuration{
			Servers: []raft.Server{
				{
					ID:      raft.ServerID(conf.Raft.NodeId),
					Address: transport.LocalAddr(),
				},
			},
		}

		raftServer.BootstrapCluster(configuration)
	}

	srv := server.New(fmt.Sprintf(conf.Server.Address), badgerDB, raftServer)
	if err := srv.Start(); err != nil {
		log.Fatal(err)
	}

	return
}
