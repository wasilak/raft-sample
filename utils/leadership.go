package utils

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/dgraph-io/badger/v2"
	"github.com/hashicorp/raft"
)

func PassLeadership(handler_raft *raft.Raft, handler_db *badger.DB) error {
	if handler_raft.State() != raft.Leader {
		err := passToLeaderAndTransferLeadership(handler_db)
		if err != nil {
			return err
		}
	} else {
		future := handler_raft.LeadershipTransfer()

		if err := future.Error(); err != nil {
			return future.Error()
		}

	}

	return nil

}

func passToLeaderAndTransferLeadership(handler_db *badger.DB) error {
	leader, errLeader := GetNodeData("CurrentLeader", handler_db)
	if errLeader != nil {
		return errors.New(errLeader.Error())
	}

	url := fmt.Sprintf("http://%s/raft/pass_leadership", leader.Server.Address)

	resp, err := http.Get(url)

	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return nil
}
