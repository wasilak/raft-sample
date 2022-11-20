package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/labstack/gommon/log"
	"github.com/wasilak/raft-sample/server/raft_handler"
	"github.com/wasilak/raft-sample/server/store_handler"
	"github.com/wasilak/raft-sample/utils"

	"github.com/dgraph-io/badger/v2"
	"github.com/hashicorp/raft"
	raftboltdb "github.com/hashicorp/raft-boltdb"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

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

// srv struct handling server
type srv struct {
	listenAddress string
	raft          *raft.Raft
	echo          *echo.Echo
}

// Start start the server
func (s srv) Start() error {
	return s.echo.StartServer(&http.Server{
		Addr:         s.listenAddress,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
	})
}

// New return new server
func New(listenAddr string, badgerDB *badger.DB, r *raft.Raft) *srv {
	e := echo.New()
	e.HideBanner = true
	e.HidePort = true
	e.Pre(middleware.RemoveTrailingSlash())
	// e.GET("/debug/pprof/*", echo.WrapHandler(http.DefaultServeMux))

	// Raft server
	raftHandler := raft_handler.New(r, badgerDB)
	e.POST("/raft/join", raftHandler.JoinRaftHandler)
	e.POST("/raft/remove", raftHandler.RemoveRaftHandler)
	e.GET("/raft/stats", raftHandler.StatsRaftHandler)
	e.GET("/raft/pass_leadership", raftHandler.PassLeadership)

	// Store server
	storeHandler := store_handler.New(r, badgerDB)
	e.POST("/store", storeHandler.Store)
	e.GET("/store/:key", storeHandler.Get)
	e.DELETE("/store/:key", storeHandler.Delete)

	return &srv{
		listenAddress: listenAddr,
		echo:          e,
		raft:          r,
	}
}

func SetupServer(conf utils.Config, raftConf *raft.Config, fsmStore raft.FSM) (*raft.Raft, *raft.NetworkTransport) {
	store, err := raftboltdb.NewBoltStore(filepath.Join(conf.Raft.VolumeDir, "raft.dataRepo"))
	if err != nil {
		log.Fatal(err)
	}

	// Wrap the store in a LogCache to improve performance.
	cacheStore, err := raft.NewLogCache(raftLogCacheSize, store)
	if err != nil {
		log.Fatal(err)
	}

	snapshotStore, err := raft.NewFileSnapshotStore(conf.Raft.VolumeDir, raftSnapShotRetain, os.Stdout)
	if err != nil {
		log.Fatal(err)
	}

	tcpAddr, err := net.ResolveTCPAddr("tcp", conf.Raft.Address)
	if err != nil {
		log.Fatal(err)
	}

	transport, err := raft.NewTCPTransport(conf.Raft.Address, tcpAddr, maxPool, tcpTimeout, os.Stdout)
	if err != nil {
		log.Fatal(err)
	}

	raftServer, err := raft.NewRaft(raftConf, fsmStore, cacheStore, store, snapshotStore, transport)
	if err != nil {
		log.Fatal(err)
	}

	return raftServer, transport
}

func BootStrapServer(conf utils.Config, raftServer *raft.Raft, transport *raft.NetworkTransport) {
	// always start single server as a leader
	configuration := raft.Configuration{
		Servers: []raft.Server{
			{
				ID:      raft.ServerID(conf.Raft.NodeId),
				Address: transport.LocalAddr(),
			},
		},
	}

	log.Debug(fmt.Sprintf("SERVER BOOTSTRAP: %+v", configuration))

	raftServer.BootstrapCluster(configuration)
}

func RetryJoin(conf utils.Config) error {
	form := raft_handler.RequestJoin{
		NodeID:      conf.Raft.NodeId,
		RaftAddress: conf.Raft.Address,
	}

	postBody, _ := json.Marshal(form)
	responseBody := bytes.NewBuffer(postBody)

	for _, peer := range conf.Peers {

		resp, err := http.Post(fmt.Sprintf("%s/raft/join", peer), "application/json", responseBody)

		//Handle Error
		if err != nil {
			log.Error(err)
			// return err
		} else {
			defer resp.Body.Close()
			if resp.StatusCode == 200 {
				return nil
			}
		}

		// else {
		// 	return fmt.Errorf("%d: %+v", resp.StatusCode, resp)
		// }

	}

	return fmt.Errorf("an error")
}

func WatchLeaderChanges(conf utils.Config, raftServer *raft.Raft, badgerDB *badger.DB) {
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
				log.Debug(fmt.Sprintf("----------------------- leader change (new leader): %+v", conf.Raft.NodeId))
				data := utils.RequestStore{
					Key:   "CurrentLeader",
					Value: conf,
				}

				log.Debug(data)

				// conf.Server.JoinAddress = conf.Server.Address

				err = utils.StoreData(data, raftServer, badgerDB)
				if err != nil {
					log.Fatal(err)
				}

				leader, errLeader := utils.GetNodeData("CurrentLeader", badgerDB)
				if errLeader != nil {
					log.Fatal(errLeader)
				}

				log.Debug(leader)
			} else {
				log.Debug(fmt.Sprintf("----------------------- leader change (not a leader): %+v", conf.Raft.NodeId))
			}

		}
	}()
}
