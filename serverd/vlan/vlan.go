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

package vlan

import (
	"code.google.com/p/gopacket"
	"code.google.com/p/gopacket/layers"
	"fmt"
	log "github.com/Sirupsen/logrus"
	"github.com/osrg/nosw/serverd/api"
	"github.com/osrg/nosw/serverd/gen-go/nosw"
	"github.com/osrg/nosw/serverd/rtmon"
	"gopkg.in/tomb.v2"
	"net"
	"os"
	"strings"

	"github.com/milosgajdos83/tenus"
)

type MacAddr [6]byte

var MAC_BROADCAST = MacAddr{0xff, 0xff, 0xff, 0xff, 0xff, 0xff}
var MAC_ZERO = MacAddr{0, 0, 0, 0, 0, 0}

func (a MacAddr) String() string {
	s := make([]string, 0)
	for _, x := range a {
		s = append(s, fmt.Sprintf("%02x", x))
	}
	return strings.Join(s, ":")
}

func NewMacAddr(hwaddr net.HardwareAddr) MacAddr {
	var a MacAddr
	copy(a[:], hwaddr[:6])
	return a
}

type Ipv4Addr [4]byte

func (a Ipv4Addr) String() string {
	s := make([]string, 0)
	for _, x := range a {
		s = append(s, fmt.Sprintf("%d", x))
	}
	return strings.Join(s, ".")
}

func NewIpv4Addr(ipaddr []byte) Ipv4Addr {
	var a Ipv4Addr
	copy(a[:], ipaddr[:4])
	return a
}

type NextHopInfo struct {
	Port uint32
	Mac  MacAddr
	GID  uint32
}

type Vlaner struct {
	t              tomb.Tomb
	api            *api.Requester
	pktInCh        chan *nosw.Packet
	vid            uint16
	mac            MacAddr
	macTable       map[MacAddr]uint32
	nextHopTable   map[Ipv4Addr]NextHopInfo
	fd, intf_index int
	ports          []uint32
}

func NewVlaner(vid uint16, ports []uint32, name string, mac net.HardwareAddr, api *api.Requester, pktInCh chan *nosw.Packet) (*Vlaner, error) {
	var v *Vlaner
	if name != "" {
		// need to keep current netns handle open to avoid PF_PACKET socket close spontaneously
		// TODO figure out why
		tenus.NetNsHandle(os.Getpid())

		index, fd, err := PFPacketBind(name)
		if err != nil {
			log.Errorf("failed to bind interface %s", name)
			return nil, err
		}
		v = &Vlaner{
			api:          api,
			pktInCh:      pktInCh,
			vid:          vid,
			ports:        ports,
			mac:          NewMacAddr(mac),
			fd:           fd,
			intf_index:   index,
			macTable:     make(map[MacAddr]uint32),
			nextHopTable: make(map[Ipv4Addr]NextHopInfo),
		}
	} else {
		v = &Vlaner{
			api:          api,
			pktInCh:      pktInCh,
			vid:          vid,
			ports:        ports,
			fd:           -1,
			intf_index:   -1,
			macTable:     make(map[MacAddr]uint32),
			nextHopTable: make(map[Ipv4Addr]NextHopInfo),
		}
	}
	return v, nil
}

func (v *Vlaner) handlePktIn() error {
	for {
		select {
		case p := <-v.pktInCh:

			pkt := gopacket.NewPacket(p.Data, layers.LayerTypeEthernet, gopacket.Default)
			ethlayer := pkt.Layer(layers.LayerTypeEthernet)
			if ethlayer == nil {
				log.Warn("no ether header")
				continue
			}
			eth, ok := ethlayer.(*layers.Ethernet)
			if ok == false {
				log.Warn("bad ethernet header")
				continue
			}

			src := NewMacAddr(eth.SrcMAC)

			if _, ok := v.macTable[src]; !ok {
				v.macTable[src] = uint32(*p.InPortNum)

				go func() {
					port := uint32(*p.InPortNum)
					err := v.api.AddL2Flow(v.vid, port, eth.SrcMAC)
					if err != nil {
						log.Errorf("failed to add l2 flow %s", eth.SrcMAC)
						return
					}

					//add L3 Flow
					var ipaddr Ipv4Addr
					if arplayer := pkt.Layer(layers.LayerTypeARP); arplayer != nil {
						arp, ok := arplayer.(*layers.ARP)
						if ok == false {
							log.Warn("bad arp packet")
							return
						}
						ipaddr = NewIpv4Addr(arp.SourceProtAddress)
					} else if iplayer := pkt.Layer(layers.LayerTypeIPv4); iplayer != nil {
						ip, ok := iplayer.(*layers.IPv4)
						if ok == false {
							log.Warn("bad ipv4 packet")
							return
						}
						ipaddr = NewIpv4Addr([]byte(ip.SrcIP))
					} else {
						log.Debug("no clue to find src IP address")
						return
					}
					gid, err := v.api.AddL3UnicastGroup(v.vid, port, net.HardwareAddr(v.mac[:]), eth.SrcMAC)
					if err != nil {
						log.Errorf("failed to add l3 unicast group %s", eth.SrcMAC)
						return
					}
					err = v.api.AddL3UnicastFlow(net.IP(ipaddr[:]), 32, gid)
					if err != nil {
						log.Errorf("failed to add l3 unicast flow %s/32. err: %s", ipaddr, err)
						return
					}

					v.nextHopTable[ipaddr] = NextHopInfo{
						Port: port,
						Mac:  src,
						GID:  gid,
					}
				}()
			}

			if v.fd > 0 && v.intf_index > 0 {
				dst := NewMacAddr(eth.DstMAC)
				target := v.mac

				if dst == target || dst == MAC_BROADCAST || target == MAC_ZERO {
					log.Debugf("packet-in | port %d, macaddr %s", *p.InPortNum, src)
					_, err := PFPacketSend(v.intf_index, v.fd, p.Data)
					if err != nil {
						log.Errorf("PFPacketSend failed")
						PFPacketClose(v.fd)
						return err
					}
				}
			}
			//			log.Debugf("packet: %s", pkt)
		case <-v.t.Dying():
			log.Debug("close PF_PACKET fd")
			PFPacketClose(v.fd)
			return nil
		}
	}
}

func (v *Vlaner) handlePktOut() error {
	errch := make(chan error)
	pktch := make(chan []byte, 1024)
	go func() {
		for {
			buf, err := PFPacketRecv(v.fd)
			if err != nil {
				log.Error("PFPacketRecv failed")
				errch <- err
			} else {
				pktch <- buf
			}
		}
	}()
	for {
		select {
		case buf := <-pktch:
			pkt := gopacket.NewPacket(buf, layers.LayerTypeEthernet, gopacket.Default)
			e := pkt.Layer(layers.LayerTypeEthernet)
			if e == nil {
				log.Warn("no ethernet header")
				continue
			}

			eth, ok := e.(*layers.Ethernet)
			if ok == false {
				log.Warn("bad ethernet header")
				continue
			}

			dst := NewMacAddr(eth.DstMAC)
			if dst == MAC_BROADCAST {
				log.Debugf("packet-out | flood")
				for _, intf := range v.ports {
					v.api.PacketOut(intf, &buf)
				}
			} else {
				intf, ok := v.macTable[dst]
				if ok == false {
					log.Warnf("unknown mac addr: %s discard..", eth.DstMAC)
					continue
				}
				log.Debugf("packet-out | port %d ", intf)
				v.api.PacketOut(intf, &buf)
			}
			//log.Debugf("packet: %s", pkt)
		case err := <-errch:
			PFPacketClose(v.fd)
			return err
		case <-v.t.Dying():
			log.Debug("close PF_PACKET fd")
			PFPacketClose(v.fd)
			return nil
		}
	}
}

func (v *Vlaner) Serve() {
	log.Debugf("start vlan%d service", v.vid)

	//recv from netns, sendto switch
	v.t.Go(v.handlePktIn)
	//recv from switch, sendto netns
	v.t.Go(v.handlePktOut)
	return
}

func (v *Vlaner) AddRoute(route *rtmon.Route) error {
	gw := NewIpv4Addr([]byte(route.Gateway))
	info, ok := v.nextHopTable[gw]
	if ok == true {
		log.Warnf("info: %v", info)
		prefixlen, _ := route.Prefix.Mask.Size()
		err := v.api.AddL3UnicastFlow(route.Prefix.IP, prefixlen, info.GID)
		if err != nil {
			log.Errorf("failed to add l3 unicast flow %s/%d. err: %s", route.Prefix.IP, prefixlen, err)
			return err
		}
	} else {
		//TODO find nexthop by sending arp
		log.Warnf("no nexthop: %s", gw)
		return fmt.Errorf("no nexthop")
	}
	return nil
}

func (v *Vlaner) DeleteRoute(route *rtmon.Route) error {
	gw := NewIpv4Addr([]byte(route.Gateway))
	info, ok := v.nextHopTable[gw]
	if ok == true {
		log.Warnf("info: %v", info)
		prefixlen, _ := route.Prefix.Mask.Size()
		err := v.api.DeleteL3UnicastFlow(route.Prefix.IP, prefixlen, info.GID)
		if err != nil {
			log.Errorf("failed to add l3 unicast flow %s/%d. err: %s", route.Prefix.IP, prefixlen, err)
			return err
		}
	} else {
		//TODO find nexthop by sending arp
		log.Warnf("no nexthop: %s", gw)
		return fmt.Errorf("no nexthop")
	}
	return nil
}

func (v *Vlaner) Stop() {
	PFPacketClose(v.fd)
}
