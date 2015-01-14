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
//#include <sys/ioctl.h>
//#include <sys/types.h>
//#include <net/if.h>
//#include <stdlib.h>
//#include <string.h>
//#include <linux/sockios.h> /* for SIOCETHTOOL */
//#include <errno.h>
//
//#define ETHTOOL_STXCSUM		0x00000017 /* Set TX hw csum enable (ethtool_value) */
//struct ethtool_value {
//	unsigned int cmd;
//	unsigned int data;
//};
//
//
//int SetTXCSUMOff(char* devname){
//    int fd, err;
//    struct ifreq ifr;
//    struct ethtool_value eval;
//
//    memset(&ifr, 0, sizeof(ifr));
//    strncpy(&ifr.ifr_name[0], devname, IFNAMSIZ);
//    ifr.ifr_name[IFNAMSIZ-1] = 0;
//    ifr.ifr_data = (caddr_t)&eval;
//    eval.cmd = ETHTOOL_STXCSUM;
//    eval.data = 0;
//
//    fd = socket(AF_INET, SOCK_DGRAM, 0);
//    if (fd < 0){
//        perror("socket");
//	return -1 * errno;
//    }
//
//    err = ioctl(fd, SIOCETHTOOL, &ifr);
//    if(err < 0){
//        perror("socket");
//	return -1 * errno;
//    }
//
//
//    close(fd);
//    return 0;
//}
import "C"
import "fmt"

func SetTXCSUMOff(name string) error {
	ret := C.SetTXCSUMOff(C.CString(name))
	if ret < 0 {
		return fmt.Errorf("failed to turn off TX checksum offload")
	}
	return nil
}
