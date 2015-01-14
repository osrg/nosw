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

package rtmon

//#include <string.h>
//#include <unistd.h>
//
//#include <net/if.h>
//#include <linux/netlink.h>
//#include <linux/rtnetlink.h>
//
//#include <sys/socket.h>
//#include <sys/types.h>
//
//#include <errno.h>
//
//int CreateRTNetlinkSocket(){
//    int err;
//    struct sockaddr_nl saddr;
//
//    int sock = socket(AF_NETLINK, SOCK_DGRAM, NETLINK_ROUTE);
//    if (sock < 0){
//	return errno * -1;
//    }
//
//    bzero(&saddr, sizeof(saddr));
//    saddr.nl_family = AF_NETLINK;
//    saddr.nl_pid = getpid();
//    saddr.nl_groups = RTMGRP_LINK | RTMGRP_IPV4_ROUTE | RTMGRP_NEIGH | RTMGRP_IPV4_IFADDR;
//    if (err = bind(sock, (struct sockaddr *) &saddr, sizeof(saddr)))
//    {
//        return errno * -1;
//    }
//    return sock;
//}
//int RTNetlinkRecv(int pd, void* buf, int len){
//    int cnt;
//    cnt = recv(pd, buf, len, 0);
//    if(cnt < 0){
//	return -1 * errno;
//    }else{
//	return cnt;
//    }
//}
//void RTNetlinkClose(int fd){
//	close(fd);
//}
import "C"

import (
	log "github.com/Sirupsen/logrus"
	"net"
	"syscall"
	"unsafe"
)

type RequestType uint8

const (
	_ RequestType = iota
	ADD_ROUTE
	DEL_ROUTE
	ADD_NEIGH
	DEL_NEIGH
)

type Route struct {
	Prefix  net.IPNet
	Gateway net.IP
}

func NewRoute(msg syscall.NetlinkMessage) (*Route, error) {
	attrs, err := syscall.ParseNetlinkRouteAttr(&msg)
	if err != nil {
		return nil, err
	}
	rtmsg := (*syscall.RtMsg)(unsafe.Pointer(&msg.Data[0]))
	route := &Route{}
	for _, attr := range attrs {
		switch RTATTRTYPE(attr.Attr.Type) {
		case RTA_DST:
			ip := attr.Value
			ipnet := net.IPNet{
				IP:   ip,
				Mask: net.CIDRMask(int(rtmsg.Dst_len), 8*len(ip)),
			}
			route.Prefix = ipnet
		case RTA_GATEWAY:
			gw := net.IP(attr.Value)
			route.Gateway = gw
		}
	}
	return route, nil
}

type Request struct {
	Event RequestType
	Data  interface{}
}

type RTMonitor struct {
	reqCh chan *Request
}

func NewRTMonitor(reqCh chan *Request) (*RTMonitor, error) {
	return &RTMonitor{
		reqCh: reqCh,
	}, nil
}

func createRTNetlinkSocket() (int, error) {
	fd := C.CreateRTNetlinkSocket()
	if fd < 0 {
		err := syscall.Errno(fd * -1)
		return 0, err
	}
	return int(fd), nil
}

func rtNetlinkRecv(fd int) ([]byte, error) {
	buf := make([]byte, 4096)
	size := C.RTNetlinkRecv(C.int(fd), unsafe.Pointer(&buf[0]), C.int(len(buf)))
	if size < 0 {
		err := syscall.Errno(size * -1)
		return nil, err
	}
	return buf[:size], nil
}

func (m *RTMonitor) Serve() error {
	fd, err := createRTNetlinkSocket()
	if err != nil {
		log.Debug("failed to create socket")
		return err
	}

	ch := make(chan []byte, 8)

	go func() {
		for {
			buf, err := rtNetlinkRecv(fd)
			if err != nil {
				log.Debug("failed to recv")
				return
			}
			ch <- buf
		}
	}()

	go func() {
		for {
			select {
			case buf := <-ch:
				msgs, err := syscall.ParseNetlinkMessage(buf)
				if err != nil {
					log.Errorf("failed to parse. %s", err)
					continue
				}
				for _, msg := range msgs {
					switch RTMSGTYPE(msg.Header.Type) {
					case RTM_NEWROUTE:
						m.handleNewRoute(msg)
					case RTM_DELROUTE:
						m.handleDelRoute(msg)
					}
				}
			}
		}
	}()

	return nil
}

func (m *RTMonitor) handleDelRoute(msg syscall.NetlinkMessage) error {
	route, err := NewRoute(msg)
	if err != nil {
		return err
	}
	m.reqCh <- &Request{
		Event: DEL_ROUTE,
		Data:  route,
	}
	return nil
}
func (m *RTMonitor) handleNewRoute(msg syscall.NetlinkMessage) error {
	route, err := NewRoute(msg)
	if err != nil {
		return err
	}
	m.reqCh <- &Request{
		Event: ADD_ROUTE,
		Data:  route,
	}
	return nil
}
