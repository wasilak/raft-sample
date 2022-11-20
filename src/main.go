package main

import (
	"flag"
	"fmt"

	"github.com/wasilak/raft-sample/server"
	"github.com/wasilak/raft-sample/utils"

	"github.com/labstack/gommon/log"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

var err error

func setConfig() utils.Config {
	conf := utils.Config{
		Server: utils.ConfigServer{
			Address:     viper.GetString("SERVER_ADDRESS"),
			JoinAddress: viper.GetString("SERVER_JOIN_ADDRESS"),
			IsServer:    viper.GetBool("IS_SERVER"),
		},
		Raft: utils.ConfigRaft{
			NodeId:    viper.GetString("RAFT_NODE_ID"),
			Address:   viper.GetString("RAFT_ADDRESS"),
			VolumeDir: viper.GetString("RAFT_VOL_DIR"),
		},
		Peers: viper.GetStringSlice("peers"),
	}

	log.Debug(fmt.Sprintf("%+v", conf))

	return conf
}

// main entry point of application start
func main() {

	viper.SetDefault("IS_SERVER", false)

	flag.Bool("verbose", false, "verbose")
	// flag.Bool("cacheEnabled", false, "cache enabled")
	// flag.String("listen", "127.0.0.1:3000", "listen address")
	flag.String("config", "./", "path to raft.yml")

	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)
	pflag.Parse()
	viper.BindPFlags(pflag.CommandLine)

	viper.SetEnvPrefix("raft")
	viper.AutomaticEnv()

	viper.SetConfigName("raft")                    // name of config file (without extension)
	viper.SetConfigType("yaml")                    // REQUIRED if the config file does not have the extension in the name
	viper.AddConfigPath(viper.GetString("config")) // path to look for the config file in
	viperErr := viper.ReadInConfig()               // Find and read the config file

	if viperErr != nil { // Handle errors reading the config file
		log.Fatal(viperErr)
		panic(viperErr)
	}

	if err != nil { // Handle errors reading the config file
		log.Fatal(err)
		panic(err)
	}

	log.Debug(viper.AllSettings())

	if viper.GetBool("debug") {
		log.SetLevel(log.DEBUG)
	}

	conf := setConfig()

	server.SetupMemberlist(conf)
	if conf.Server.IsServer {
		badgerDB, raftServer := server.SetupRaft(conf)

		srv := server.New(fmt.Sprintf(conf.Server.Address), badgerDB, raftServer)
		if err := srv.Start(); err != nil {
			log.Fatal(err)
		}
	}
}
