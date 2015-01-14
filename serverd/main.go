// Copyright (C) 2014 Nippon Telegraph and Telephone Corporation.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or
// implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	log "github.com/Sirupsen/logrus"
	"github.com/jessevdk/go-flags"
	"github.com/osrg/nosw/serverd/config"
	"github.com/osrg/nosw/serverd/gen-go/nosw"
	"github.com/osrg/nosw/serverd/pktin"
	"github.com/osrg/nosw/serverd/vlan"
	"os"
	"os/signal"
	"runtime"
	"syscall"
)

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT)

	var opts struct {
		ConfigFile string `short:"f" long:"config-file" description:"specifying a config file"`
		LogLevel   string `short:"l" long:"log-level" description:"specifying log level"`
		LogJson    bool   `short:"j" long:"log-json" description:"use json format for logging"`
	}
	_, err := flags.Parse(&opts)
	if err != nil {
		os.Exit(1)
	}

	switch opts.LogLevel {
	case "debug":
		log.SetLevel(log.DebugLevel)
	case "info":
		log.SetLevel(log.InfoLevel)
	default:
		log.SetLevel(log.InfoLevel)
	}
	log.SetOutput(os.Stderr)
	if opts.LogJson {
		log.SetFormatter(&log.JSONFormatter{})
	}

	if opts.ConfigFile == "" {
		opts.ConfigFile = "serverd.conf"
	}

	configCh := make(chan config.SwitchType)
	reloadCh := make(chan bool)
	go config.ReadConfigfileServe(opts.ConfigFile, configCh, reloadCh)
	reloadCh <- true

	var manager *vlan.Manager
	var switchConfig *config.SwitchType = nil

	for {
		select {
		case newConfig := <-configCh:
			var added []config.VlanType
			var deleted []config.VlanType

			if switchConfig == nil {
				g := newConfig.Global
				pktInChMap := make(map[uint16]chan *nosw.Packet)

				pktInServer, _ := pktin.NewServer(g.ServerdPort, pktInChMap)
				log.Infof("start packet-in server. listen on %d", g.ServerdPort)
				go pktInServer.Serve()

				manager, _ = vlan.NewManager(g, pktInChMap)
				log.Infof("start vlan manager")
				go manager.Serve()

				switchConfig = &newConfig
				added = newConfig.VlanList
				deleted = []config.VlanType{}
			} else {
				log.Error("online config update is not supported yet")
			}

			for _, v := range added {
				log.Infof("Vlan %d is added", v.Vid)
				manager.VlanAdd(v)
			}

			for _, v := range deleted {
				log.Infof("Vlan %d is deleted", v.Vid)
				manager.VlanDelete(v)
			}
		case sig := <-sigCh:
			switch sig {
			case syscall.SIGINT:
				manager.CleanUp()
				os.Exit(1)
			}
		}
	}
}
