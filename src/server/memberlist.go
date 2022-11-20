package server

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/hashicorp/memberlist"
	"github.com/labstack/gommon/log"
	"github.com/wasilak/raft-sample/utils"
)

var list *memberlist.Memberlist

func SetupMemberlist(conf utils.Config) {
	list, err = memberlist.Create(memberlist.DefaultLocalConfig())
	if err != nil {
		panic("Failed to create memberlist: " + err.Error())
	}

	log.Debug(list)

	JoinMemberlist(conf)

	GetMemberlistMembers()
}

// Join an existing cluster by specifying at least one known member.
func JoinMemberlist(conf utils.Config) {
	var membersToJoin []string

	for _, address := range conf.Peers {
		url, err := url.Parse(address)
		if err != nil {
			log.Fatal(err)
		}
		addr := strings.TrimPrefix(url.Hostname(), "www.")
		membersToJoin = append(membersToJoin, addr)
	}
	n, err := list.Join(membersToJoin)
	if err != nil {
		panic("Failed to join cluster: " + err.Error())
	}

	log.Debug(fmt.Sprintf("Hosts contacted: %d", n))
}

// Ask for members of the cluster
func GetMemberlistMembers() {
	for _, member := range list.Members() {
		fmt.Printf("Member: %s %s\n", member.Name, member.Addr)
	}
}
