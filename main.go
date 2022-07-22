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
	"sync/atomic"
	"time"

	"github.com/wasilak/raft-sample/fsm"
	"github.com/wasilak/raft-sample/server"
	"github.com/wasilak/raft-sample/server/raft_handler"
	"github.com/wasilak/raft-sample/utils"

	"github.com/dgraph-io/badger/v2"
	"github.com/hashicorp/raft"
	raftboltdb "github.com/hashicorp/raft-boltdb"
	"github.com/spf13/viper"
)

const (
	serverAddress     = "SERVER_ADDRESS"
	serverJoinAddress = "SERVER_JOIN_ADDRESS"

	serverBootstrap = "IS_SERVER"

	raftNodeId  = "RAFT_NODE_ID"
	raftAddress = "RAFT_ADDRESS"
	raftVolDir  = "RAFT_VOL_DIR"
)

var confKeys = []string{
	serverAddress,
	serverJoinAddress,
	serverBootstrap,

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

func retryJoin(conf utils.Config) int {
	form := raft_handler.RequestJoin{
		NodeID:      conf.Raft.NodeId,
		RaftAddress: conf.Raft.Address,
	}

	postBody, _ := json.Marshal(form)
	responseBody := bytes.NewBuffer(postBody)

	resp, err := http.Post(fmt.Sprintf("%s/raft/join", conf.Server.JoinAddress), "application/json", responseBody)

	//Handle Error
	if err != nil {
		log.Fatal(err)
	}

	defer resp.Body.Close()

	return resp.StatusCode
}

// main entry point of application start
func main() {

	var v = viper.New()
	v.AutomaticEnv()
	if err := v.BindEnv(confKeys...); err != nil {
		log.Fatal(err)
		return
	}

	viper.SetDefault(serverBootstrap, false)

	conf := utils.Config{
		Server: utils.ConfigServer{
			Address:         v.GetString(serverAddress),
			JoinAddress:     v.GetString(serverJoinAddress),
			ServerBootstrap: v.GetBool(serverBootstrap),
		},
		Raft: utils.ConfigRaft{
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

	if conf.Server.ServerBootstrap {

		// always start single server as a leader
		configuration := raft.Configuration{
			Servers: []raft.Server{
				{
					ID:      raft.ServerID(conf.Raft.NodeId),
					Address: transport.LocalAddr(),
				},
			},
		}

		log.Println(configuration)

		raftServer.BootstrapCluster(configuration)
	}

	if len(conf.Server.JoinAddress) > 0 {

		for {
			statusCode := retryJoin(conf)
			time.Sleep(4 * time.Second)
			if statusCode == 200 {
				break
			}
		}
	}

	data := utils.RequestStore{
		Key:   fmt.Sprintf("member_%s", conf.Raft.NodeId),
		Value: conf,
	}

	utils.StoreData(data, raftServer, badgerDB)

	seeNewLeader := func(o *raft.Observation) bool { _, ok := o.Data.(raft.LeaderObservation); return ok }
	leaderCh := make(chan raft.Observation)

	raftServer.RegisterObserver(raft.NewObserver(leaderCh, false, seeNewLeader))

	leaderChanges := new(uint32)
	go func() {
		// for elem := range leaderCh {
		for range leaderCh {
			atomic.AddUint32(leaderChanges, 1)
			// elemVal := elem.Data.(raft.LeaderObservation)

			if raftServer.State() == raft.Leader {
				data := utils.RequestStore{
					Key:   "CurrentLeader",
					Value: conf,
				}

				utils.StoreData(data, raftServer, badgerDB)
			}

		}
	}()

	srv := server.New(fmt.Sprintf(conf.Server.Address), badgerDB, raftServer)
	if err := srv.Start(); err != nil {
		log.Fatal(err)
	}

	return
}
