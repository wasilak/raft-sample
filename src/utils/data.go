package utils

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/dgraph-io/badger/v2"
	"github.com/hashicorp/raft"
	"github.com/wasilak/raft-sample/fsm"
)

// RequestStore payload for storing new data in raft cluster
type RequestStore struct {
	Key   string      `json:"key"`
	Value interface{} `json:"value"`
}

func StoreData(data RequestStore, handler_raft *raft.Raft, handler_db *badger.DB) error {

	if handler_raft.State() != raft.Leader {
		passToLeaderAndStore(data, handler_db)
	} else {
		err := storeOnLeadeer(data, handler_raft)

		if err != nil {
			return err
		}
	}

	return nil
}

func passToLeaderAndStore(data RequestStore, handler_db *badger.DB) error {
	postBody, _ := json.Marshal(data)
	responseBody := bytes.NewBuffer(postBody)

	leader, errLeader := GetNodeData("CurrentLeader", handler_db)
	if errLeader != nil {
		return errors.New(errLeader.Error())
	}

	url := fmt.Sprintf("http://%s/store", leader.Server.Address)

	resp, err := http.Post(url, "application/json", responseBody)

	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return nil
}

func storeOnLeadeer(req RequestStore, handler_raft *raft.Raft) error {
	payload := fsm.CommandPayload{
		Operation: "SET",
		Key:       req.Key,
		Value:     req.Value,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return errors.New(fmt.Sprintf("error preparing saving data payload: %s", err.Error()))
	}

	applyFuture := handler_raft.Apply(data, 500*time.Millisecond)
	if err := applyFuture.Error(); err != nil {
		return errors.New(fmt.Sprintf("error persisting data in raft cluster: %s", err.Error()))
	}

	_, ok := applyFuture.Response().(*fsm.ApplyResponse)
	if !ok {
		return errors.New(fmt.Sprintf("error response is not match apply response"))
	}

	return nil
}

func GetData(key string, handler_db *badger.DB) (interface{}, error) {
	txn := handler_db.NewTransaction(false)
	defer func() {
		_ = txn.Commit()
	}()

	value, err := actualGetData(key, txn)

	// var data utils.Config
	var data interface{}
	if value != nil && len(value) > 0 {
		err = json.Unmarshal(value, &data)
	}

	if err != nil {
		return data, errors.New(fmt.Sprintf("error unmarshal data to interface: %s", err.Error()))
	}

	return data, nil
}

func GetNodeData(key string, handler_db *badger.DB) (Config, error) {
	txn := handler_db.NewTransaction(false)
	defer func() {
		_ = txn.Commit()
	}()

	value, err := actualGetData(key, txn)

	var data Config
	if value != nil && len(value) > 0 {
		err = json.Unmarshal(value, &data)
	}

	if err != nil {
		return data, errors.New(fmt.Sprintf("error unmarshal data to interface: %s", err.Error()))
	}

	return data, nil
}

func actualGetData(key string, txn *badger.Txn) ([]byte, error) {
	var keyByte = []byte(key)

	item, err := txn.Get(keyByte)
	if err != nil {
		return nil, errors.New(fmt.Sprintf("error getting key %s from storage: %s", key, err.Error()))
	}

	var value = make([]byte, 0)
	err = item.Value(func(val []byte) error {
		value = append(value, val...)
		return nil
	})

	if err != nil {
		return nil, errors.New(fmt.Sprintf("error appending byte value of key %s from storage: %s", key, err.Error()))
	}

	return value, nil
}
