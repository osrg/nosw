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

package pktin

import (
	"code.google.com/p/gopacket"
	"code.google.com/p/gopacket/layers"
	"fmt"
	log "github.com/Sirupsen/logrus"
	"github.com/apache/thrift/lib/go/thrift"
	"github.com/osrg/nosw/serverd/gen-go/nosw"
)

type handler struct {
	chMap map[uint16]chan *nosw.Packet
}

func (h handler) TossPacket(pkt *nosw.Packet) error {
	p := gopacket.NewPacket(pkt.Data, layers.LayerTypeEthernet, gopacket.Default)
	vlayer := p.Layer(layers.LayerTypeDot1Q)

	if vlayer == nil {
		log.Warnf("no vlan header")
		return nil
	}

	vlan, _ := vlayer.(*layers.Dot1Q)
	vid := vlan.VLANIdentifier
	_, ok := h.chMap[vid]
	if ok == false {
		log.Warnf("pktin chan for %d is not ready!", vid)
		return nil
	}

	h.chMap[vid] <- pkt

	return nil
}

type Server struct {
	server thrift.TServer
}

func NewServer(port uint16, chMap map[uint16]chan *nosw.Packet) (*Server, error) {
	host := fmt.Sprintf("%s:%d", "0.0.0.0", port)
	s_trans, err := thrift.NewTServerSocket(host)
	if err != nil {
		log.Error("failed to listen")
		return nil, err
	}
	processor := nosw.NewEscalatorProcessor(handler{chMap})
	s := thrift.NewTSimpleServer2(processor, s_trans)
	return &Server{
		server: s,
	}, nil
}

func (s *Server) Serve() {
	s.server.Serve()
}
