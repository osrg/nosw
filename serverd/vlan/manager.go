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
	"fmt"
	log "github.com/Sirupsen/logrus"
	"github.com/milosgajdos83/tenus"
	"github.com/osrg/nosw/serverd/api"
	"github.com/osrg/nosw/serverd/config"
	"github.com/osrg/nosw/serverd/gen-go/nosw"
	"github.com/osrg/nosw/serverd/rtmon"
	"net"
	"time"
)

type vlanMapInfo struct {
	vlaner    *Vlaner
	intf_name string
	config    config.VlanType
}

type Manager struct {
	config        config.GlobalType
	api           *api.Requester
	pktInChMap    map[uint16]chan *nosw.Packet
	addedVlanCh   chan config.VlanType
	deletedVlanCh chan config.VlanType
	rtReqCh       chan *rtmon.Request
	vlanMap       map[uint16]vlanMapInfo
}

func NewManager(c config.GlobalType, pktInChMap map[uint16]chan *nosw.Packet) (*Manager, error) {
	reqCh := make(chan *api.Request, 1024)
	host := fmt.Sprintf("%s:%d", c.SwitchdIP, c.SwitchdPort)

	requester, err := api.NewRequester(host, reqCh)
	if err != nil {
		log.Debugf("failed to create requester")
		return nil, err
	}
	log.Infof("start requester. connect to %s", host)
	go requester.Serve()

	rtreqCh := make(chan *rtmon.Request, 8)

	rtmon, err := rtmon.NewRTMonitor(rtreqCh)
	if err != nil {
		log.Debugf("failed to create rtmonitor")
		return nil, err
	}
	log.Infof("start rtmonitor")
	go rtmon.Serve()

	m := Manager{}
	m.config = c
	m.api = requester
	m.pktInChMap = pktInChMap
	m.addedVlanCh = make(chan config.VlanType)
	m.deletedVlanCh = make(chan config.VlanType)
	m.rtReqCh = rtreqCh
	m.vlanMap = make(map[uint16]vlanMapInfo)
	return &m, nil
}

func (m *Manager) createVLANInterface(vid uint16, host string) (string, net.HardwareAddr, error) {
	name1 := fmt.Sprintf("veth%d-1", vid)
	name2 := fmt.Sprintf("veth%d-2", vid)
	name3 := fmt.Sprintf("eth%d", vid)

	log.Debugf("create veth pair veth1:%s, veth2:%s", name1, name2)
	veth, err := tenus.NewVethPairWithOptions(name1, tenus.VethOptions{PeerName: name2})
	if err != nil {
		return "", nil, err
	}

	log.Debugf("link up %s", name1)
	if err := veth.SetLinkUp(); err != nil {
		return "", nil, err
	}
	log.Debugf("link up %s", name2)
	if err := veth.SetPeerLinkUp(); err != nil {
		return "", nil, err
	}

	mtu := 4000

	log.Debugf("set %s mtu to %d", name1, mtu)
	if err := veth.SetLinkMTU(mtu); err != nil {
		log.Errorf("failed to set mtu to %d", mtu)
		return "", nil, err
	}

	log.Debugf("set %s mtu to %d", name2, mtu)
	if err := veth.SetPeerLinkMTU(mtu); err != nil {
		log.Errorf("failed to set mtu to %d", mtu)
		return "", nil, err
	}

	//need to turn off TX checksum offloading
	//TODO figure out why
	if err := SetTXCSUMOff(name1); err != nil {
		log.Errorf("failed to turn off TX checksum offload: %s", name1)
		return "", nil, err
	}

	//need to turn off TX checksum offloading
	//TODO figure out why
	if err := SetTXCSUMOff(name2); err != nil {
		log.Errorf("failed to turn off TX checksum offload: %s", name2)
		return "", nil, err
	}

	log.Debugf("create vlan interface: %s on top of %s", name3, name2)
	vlan, err := tenus.NewVlanLinkWithOptions(name2, tenus.VlanOptions{Dev: name3, Id: vid})
	if err != nil {
		return "", nil, err
	}

	mtu = 1500

	log.Debugf("set %s mtu to %d", name3, mtu)
	if err := vlan.SetLinkMTU(mtu); err != nil {
		log.Errorf("failed to set mtu to %d", mtu)
		return "", nil, err
	}

	log.Debugf("link up %s", name3)
	if err := vlan.SetLinkUp(); err != nil {
		return "", nil, err
	}

	log.Debugf("set IP Address: %s for %s", host, name3)
	ip, ipnet, err := net.ParseCIDR(host)
	if err != nil {
		return "", nil, err
	}

	if err := vlan.SetLinkIp(ip, ipnet); err != nil {
		return "", nil, err
	}

	return name1, vlan.NetInterface().HardwareAddr, nil
}

func (m *Manager) deleteVLANInterface(name string) error {
	_, err := net.InterfaceByName(name)
	if err != nil {
		log.Debugf("no interface %s found", name)
		return err
	}
	log.Debugf("delete link %s", name)
	return tenus.DeleteLink(name)
}

func (m *Manager) purgeConfig() error {
	flows, err := m.api.GetVLANFlows()
	if err != nil {
		log.Debugf("failed to get VLAN flows")
		return err
	}

	f, err := m.api.GetL2Flows()
	if err != nil {
		log.Debugf("failed to get L2 flows")
		return err
	}
	flows = append(flows, f...)

	f, err = m.api.GetACLFlows()
	if err != nil {
		log.Debugf("failed to get ACL flows")
		return err
	}
	flows = append(flows, f...)

	f, err = m.api.GetTermMACFlows()
	if err != nil {
		log.Debugf("failed to get MAC Termination flows")
		return err
	}
	flows = append(flows, f...)

	f, err = m.api.GetL3UnicastFlows()
	if err != nil {
		log.Debugf("failed to get MAC Termination flows")
		return err
	}
	flows = append(flows, f...)

	for _, f := range flows {
		if err := m.api.DeleteFlow(f); err != nil {
			log.Debugf("failed to delete flow: %s", f)
			return err
		}
	}

	groups, err := m.api.GetGroups()
	if err != nil {
		log.Debugf("failed to get groups")
		return err
	}

	for _, g := range groups {
		log.Debugf("delete group: %s", g)
		m.api.DeleteGroup(g)
	}

	groups, err = m.api.GetGroups()
	if err != nil {
		log.Debugf("failed to get groups")
		return err
	}

	for _, g := range groups {
		log.Debugf("delete group: %s", g)
		err = m.api.DeleteGroup(g)
		if err != nil {
			log.Debugf("failed to delete group")
			return err
		}
	}

	return nil
}

func (m *Manager) Serve() error {
	err := m.purgeConfig()
	if err != nil {
		log.Errorf("failed to purge config: %v", err)
		return fmt.Errorf("failed to purge config")
	}

	for {
		select {
		case c := <-m.addedVlanCh:
			log.Debugf("%s", c)
			fgid, err := m.api.AddL2FloodGroup(c.Vid)
			if err != nil {
				log.Debugf("%s", err)
			}
			for i, p := range c.Ports {
				log.Debugf("add flow for %d", p)
				err := m.api.AddVLANFlow(c.Vid, p, true)
				if err != nil {
					log.Debugf("%s", err)
				}
				err = m.api.AddVLANFlow(c.Vid, p, false)
				if err != nil {
					log.Debugf("%s", err)
				}
				gid, err := m.api.AddL2InterfaceGroup(c.Vid, p)
				if err != nil {
					log.Debugf("%s", err)
				}
				err = m.api.AddL2InterfaceBucket(gid, p, true)
				if err != nil {
					log.Debugf("%s", err)
				}
				err = m.api.AddL2FloodBucket(fgid, gid, i)
				if err != nil {
					log.Debugf("%s", err)
				}
			}
			err = m.api.AddL2FloodFlow(c.Vid)
			if err != nil {
				log.Debugf("%s", err)
			}

			var name string
			var mac net.HardwareAddr
			if c.Host != "" {
				log.Debugf("create VLAN interface")
				name, mac, err = m.createVLANInterface(c.Vid, c.Host)
				if err != nil {
					log.Errorf("failed to create vlan interface for vid: %d, err: %s", c.Vid, err)
					continue
				}

				ip, _, _ := net.ParseCIDR(c.Host)

				err = m.api.AddHostRegistrationFlow(c.Vid, ip)
				if err != nil {
					log.Errorf("failed to regist host %s(vid: %d)", ip, c.Vid)
					continue
				}

				err = m.api.AddTerminationMacFlow(c.Vid, mac)
				if err != nil {
					log.Errorf("failed to add MAC termination flow(mac: %s), err", mac, err)
					continue
				}
			}

			pktinCh := make(chan *nosw.Packet, 1024)
			m.pktInChMap[c.Vid] = pktinCh

			vlaner, err := NewVlaner(c.Vid, c.Ports, name, mac, m.api, pktinCh)
			if err != nil {
				log.Errorf("failed to create VLAN handler for vid %d", c.Vid)
				continue
			}

			m.vlanMap[c.Vid] = vlanMapInfo{
				vlaner:    vlaner,
				intf_name: name,
				config:    c,
			}

			go vlaner.Serve()

		case c := <-m.deletedVlanCh:
			vid := c.Vid
			info, found := m.vlanMap[vid]
			if found {
				log.Info("Delete a vlan configuration for ", vid)
				info.vlaner.Stop()
				delete(m.vlanMap, vid)
			} else {
				log.Info("Can't delete a vlan configuration for ", vid)
			}

			err := m.deleteVLANInterface(info.intf_name)
			if err != nil {
				log.Errorf("failed to delete vlan interface %d", vid)
			}
		case c := <-m.rtReqCh:
			switch c.Event {
			case rtmon.ADD_ROUTE:
				route, ok := c.Data.(*rtmon.Route)
				if ok == false {
					log.Errorf("ADD_ROUTE: failed to cast Data to Route")
					continue
				}
				vid := m.findVidFromGateway(route.Gateway)
				info, found := m.vlanMap[vid]
				if found {
					err := info.vlaner.AddRoute(route)
					if err != nil {
						log.Errorf("failed to add route %s", route.Prefix)
					}
				} else {
					log.Errorf("Can't find vlaner from gateway %s", route.Gateway)
				}
			case rtmon.DEL_ROUTE:
				route, ok := c.Data.(*rtmon.Route)
				if ok == false {
					log.Errorf("DEL_ROUTE: failed to cast Data to Route")
					continue
				}
				vid := m.findVidFromGateway(route.Gateway)
				info, found := m.vlanMap[vid]
				if found {
					err := info.vlaner.DeleteRoute(route)
					if err != nil {
						log.Errorf("failed to delete route %s", route.Prefix)
					}
				} else {
					log.Errorf("Can't find vlaner from gateway %s", route.Gateway)
				}
			}
		}
	}
	return nil
}

func (m *Manager) findVidFromGateway(gw net.IP) uint16 {
	for vid, info := range m.vlanMap {
		_, ipnet, err := net.ParseCIDR(info.config.Host)
		if err != nil {
			return 0
		}
		if ipnet.Contains(gw) == true {
			return vid
		}
	}
	return 0
}

func (m *Manager) VlanAdd(vlan config.VlanType) {
	m.addedVlanCh <- vlan
}

func (m *Manager) VlanDelete(vlan config.VlanType) {
	m.deletedVlanCh <- vlan
}

func (m *Manager) CleanUp() {
	for _, v := range m.vlanMap {
		m.VlanDelete(v.config)
	}
	time.Sleep(1000 * 1000 * 1000 * 1) // wait for deletion
}
