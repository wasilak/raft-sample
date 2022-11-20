package server

import (
	"fmt"
	// _ "net/http/pprof"
	"os"
	"time"

	"github.com/labstack/gommon/log"
	"github.com/wasilak/raft-sample/fsm"
	"github.com/wasilak/raft-sample/utils"

	"github.com/dgraph-io/badger/v2"
	"github.com/hashicorp/raft"
)

var err error

func setRaftConfig(conf utils.Config) *raft.Config {
	raftConf := raft.DefaultConfig()
	raftConf.LocalID = raft.ServerID(conf.Raft.NodeId)
	raftConf.SnapshotThreshold = 1024

	return raftConf
}

func setFsmStore(badgerDB *badger.DB) raft.FSM {
	fsmStore := fsm.NewBadger(badgerDB)

	return fsmStore
}

func setupBadgerDB(conf utils.Config) *badger.DB {
	// Preparing badgerDB
	badgerOpt := badger.DefaultOptions(conf.Raft.VolumeDir)
	badgerDB, err := badger.Open(badgerOpt)
	if err != nil {
		log.Fatal(err)
	}

	defer func() {
		if err := badgerDB.Close(); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "error close badgerDB: %s\n", err.Error())
		}
	}()

	return badgerDB
}

func SetupRaft(conf utils.Config) (*badger.DB, *raft.Raft) {
	badgerDB := setupBadgerDB(conf)

	raftConf := setRaftConfig(conf)

	fsmStore := setFsmStore(badgerDB)

	raftServer, transport := SetupServer(conf, raftConf, fsmStore)

	BootStrapServer(conf, raftServer, transport)

	if len(conf.Peers) > 0 {

		go func() {
			for {
				err = RetryJoin(conf)
				if err == nil {
					break
				}
				time.Sleep(2 * time.Second)
			}
		}()
	}

	data := utils.RequestStore{
		Key:   fmt.Sprintf("member_%s", conf.Raft.NodeId),
		Value: conf,
	}

	utils.StoreData(data, raftServer, badgerDB)

	WatchLeaderChanges(conf, raftServer, badgerDB)

	return badgerDB, raftServer
}

func Leave(conf utils.Config, raftServer *raft.Raft) error {
	future := raftServer.RemoveServer(raft.ServerID(conf.Raft.NodeId), 0, 0)
	if err := future.Error(); err != nil {
		return err
	}

	return nil
}
