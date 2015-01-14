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

//#include <sys/socket.h>
//#include <netpacket/packet.h>
//#include <net/ethernet.h>
//#include <sys/ioctl.h>
//#include <net/if.h>
//#include <string.h>
//#include <stdio.h>
//#include <errno.h>
//int PFPacketBind(char* name, int* intf_index, int* pd){
//    struct ifreq ifr;
//    struct sockaddr_ll sll;
//    struct packet_mreq mreq;
//    int err = 0;
//
//    *pd = socket(PF_PACKET, SOCK_RAW, htons(ETH_P_ALL));
//
//    memset(&ifr, 0, sizeof(ifr));
//    strncpy(ifr.ifr_name, name, IFNAMSIZ);
//    ioctl(*pd, SIOCGIFINDEX, &ifr);
//    *intf_index = ifr.ifr_ifindex;
//
//    memset(&mreq, 0, sizeof(mreq));
//    mreq.mr_type = PACKET_MR_PROMISC;
//    mreq.mr_ifindex = ifr.ifr_ifindex;
//
//    if((setsockopt(*pd,SOL_PACKET,PACKET_ADD_MEMBERSHIP, (void *)&mreq, sizeof(mreq))) < 0 ){
//        perror("setsockopt");
//        return -1 * errno;
//    }
//
//    memset(&sll, 0xff, sizeof(sll));
//    sll.sll_family = AF_PACKET;
//    sll.sll_protocol = htons(ETH_P_ALL);
//    sll.sll_ifindex = *intf_index;
//    err = bind(*pd, (struct sockaddr *)&sll, sizeof sll);
//    return err;
//}
//int PFPacketSend(int intf_index, int pd, void* buf, int len){
//    struct sockaddr_ll sll;
//    int cnt;
//    memset(&sll, 0, sizeof(sll));
//    sll.sll_ifindex = intf_index;
//    cnt = sendto(pd, buf, len, 0, (struct sockaddr *)&sll, sizeof sll);
//    if(cnt < 0){
//	return -1 * errno;
//    }else{
//	return cnt;
//    }
//}
//int PFPacketRecv(int pd, void* buf, int len){
//    int cnt;
//    cnt = recv(pd, buf, len, 0);
//    if(cnt < 0){
//	return -1 * errno;
//    }else{
//	return cnt;
//    }
//}
//void PFPacketClose(int fd){
//	close(fd);
//}
import "C"
import (
	"fmt"
	log "github.com/Sirupsen/logrus"
	"syscall"
	"unsafe"
)

func PFPacketBind(name string) (int, int, error) {
	var index, fd C.int
	ret := C.PFPacketBind(C.CString(name), &index, &fd)
	if ret < 0 {
		return 0, 0, fmt.Errorf("failed to bind with %s", name)
	}
	return int(index), int(fd), nil
}

func PFPacketClose(fd int) {
	C.PFPacketClose(C.int(fd))
}

func PFPacketRecv(fd int) ([]byte, error) {
	//TODO get mtu by request
	buf := make([]byte, 2000)
	size := C.PFPacketRecv(C.int(fd), unsafe.Pointer(&buf[0]), C.int(len(buf)))
	if size < 0 {
		err := syscall.Errno(size * -1)
		log.Errorf("failed to send a packet. err: %s", err)
		return nil, fmt.Errorf("failed to recv a packet. size: %d", size)
	}
	return buf[:size], nil
}

func PFPacketSend(index int, fd int, buf []byte) (int, error) {
	size := C.PFPacketSend(C.int(index), C.int(fd), unsafe.Pointer(&buf[0]), C.int(len(buf)))
	if size < 0 {
		err := syscall.Errno(size * -1)
		log.Errorf("failed to send a packet. err: %s", err)
		return 0, fmt.Errorf("failed to send a packet. ret: %d", size)
	}
	return int(size), nil
}
