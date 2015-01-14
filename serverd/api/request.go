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

package api

import (
	log "github.com/Sirupsen/logrus"
	"github.com/apache/thrift/lib/go/thrift"
	"github.com/osrg/nosw/serverd/gen-go/nosw"
	"gopkg.in/tomb.v2"
	"net"
)

type RequestType uint8

const (
	_ RequestType = iota
	TOSS_PKT
	TABLE_SUPPORTED
	GET_FLOW_TABLE_INFO
	ADD_FLOW
	MOD_FLOW
	DEL_FLOW
	GET_FLOWS
	GET_FLOW_STAT
	GET_FLOW_BY_COOKIE
	DEL_FLOW_BY_COOKIE
	GET_GROUP_TABLE_INFO
	ADD_GROUP
	DEL_GROUP
	GET_GROUPS
	GET_GROUP_STAT
	ADD_BUCKET
	DEL_BUCKET
	MOD_BUCKET
	GET_BUCKETS
	GET_PORT_STATS
	CLEAR_PORT_STATS
)

type Response struct {
	Err  error
	Data interface{}
}

type Request struct {
	Event RequestType
	Data  interface{}
	ResCh chan *Response
}

func NewGetFlowsRequest(tableId nosw.TABLE_ID) *Request {
	resCh := make(chan *Response, 1)
	return &Request{
		Event: GET_FLOWS,
		Data:  tableId,
		ResCh: resCh,
	}
}

type Requester struct {
	t                     tomb.Tomb
	ReqCh                 chan *Request
	host                  string
	nextUnicastGroupIndex int64
}

func NewRequester(host string, reqCh chan *Request) (*Requester, error) {
	return &Requester{
		ReqCh: reqCh,
		host:  host,
	}, nil
}

func (r *Requester) Serve() error {
	trans, err := thrift.NewTSocket(r.host)
	if err != nil {
		log.Error("Error creating transport")
		return err
	}

	protocol := thrift.NewTBinaryProtocolFactoryDefault()
	client := nosw.NewNoswClientFactory(trans, protocol)
	if err := trans.Open(); err != nil {
		log.Errorf("Error opening socket")
		return err
	}

	r.t.Go(func() error {
		for {
			select {
			case req := <-r.ReqCh:
				log.Debugf("Req: %v", *req)
				switch req.Event {
				case TOSS_PKT:
					client.TossPacket(req.Data.(*nosw.Packet))
				case ADD_FLOW:
					err := client.AddFlow(req.Data.(*nosw.FlowEntry))
					req.ResCh <- &Response{Err: err}
				case MOD_FLOW:
					err := client.ModifyFlow(req.Data.(*nosw.FlowEntry))
					req.ResCh <- &Response{Err: err}
				case DEL_FLOW:
					err := client.DeleteFlow(req.Data.(*nosw.FlowEntry))
					req.ResCh <- &Response{Err: err}
				case GET_FLOWS:
					flows, err := client.GetFlows(req.Data.(nosw.TABLE_ID))
					req.ResCh <- &Response{Data: flows, Err: err}
				case ADD_GROUP:
					err := client.AddGroup(req.Data.(*nosw.GroupEntry))
					req.ResCh <- &Response{Err: err}
				case DEL_GROUP:
					err := client.DeleteGroup(req.Data.(*nosw.GroupEntry))
					req.ResCh <- &Response{Err: err}
				case GET_GROUPS:
					groups, err := client.GetGroups()
					req.ResCh <- &Response{Data: groups, Err: err}
				case ADD_BUCKET:
					err := client.AddBucket(req.Data.(*nosw.GroupBucketEntry))
					req.ResCh <- &Response{Err: err}
				case DEL_BUCKET:
					err := client.DeleteBucket(req.Data.(*nosw.GroupBucketEntry))
					req.ResCh <- &Response{Err: err}
				case GET_BUCKETS:
					buckets, err := client.GetBuckets(req.Data.(*nosw.GroupEntry))
					req.ResCh <- &Response{Data: buckets, Err: err}
				}
			case <-r.t.Dying():
				log.Debug("Requester Dying")
				return nil
			}
		}
	})
	return nil
}

func NewPacketOutRequest(port uint32, data *[]byte) *Request {
	p := int64(port)
	packet := nosw.Packet{
		Data:      *data,
		InPortNum: &p,
	}
	resCh := make(chan *Response, 1)
	return &Request{
		Event: TOSS_PKT,
		Data:  &packet,
		ResCh: resCh,
	}
}

func (r *Requester) PacketOut(port uint32, data *[]byte) {
	req := NewPacketOutRequest(port, data)
	r.ReqCh <- req
}

func (r *Requester) GetFlows(id nosw.TABLE_ID) ([]*nosw.FlowEntry, error) {
	req := NewGetFlowsRequest(id)
	r.ReqCh <- req
	res := <-req.ResCh
	if res.Err != nil {
		return nil, res.Err
	}
	return res.Data.([]*nosw.FlowEntry), nil
}

func (r *Requester) GetVLANFlows() ([]*nosw.FlowEntry, error) {
	return r.GetFlows(nosw.TABLE_ID_VLAN)
}

func (r *Requester) GetL2Flows() ([]*nosw.FlowEntry, error) {
	return r.GetFlows(nosw.TABLE_ID_BRIDGING)
}

func (r *Requester) GetTermMACFlows() ([]*nosw.FlowEntry, error) {
	return r.GetFlows(nosw.TABLE_ID_TERMINATION_MAC)
}

func (r *Requester) GetACLFlows() ([]*nosw.FlowEntry, error) {
	return r.GetFlows(nosw.TABLE_ID_ACL_POLICY)
}

func (r *Requester) GetL3UnicastFlows() ([]*nosw.FlowEntry, error) {
	return r.GetFlows(nosw.TABLE_ID_UNICAST_ROUTING)
}

func NewL2FlowEntry(vid uint16, port uint32, mac net.HardwareAddr, flood bool) *nosw.FlowEntry {
	mask := int32(0x1fff)
	var m nosw.BridgingFlowMatch
	var gid int64
	var outputPort int64

	if flood == false {
		m = nosw.BridgingFlowMatch{
			VlanId:     int32(vid),
			VlanIdMask: &mask,
			DstMac:     mac.String(),
			DstMacMask: "ff:ff:ff:ff:ff:ff",
		}
		gid = int64(nosw.GROUP_TYPE_L2_INTERFACE<<28) | int64(vid)<<16 | int64(port)
	} else {
		m = nosw.BridgingFlowMatch{
			VlanId:     int32(vid),
			VlanIdMask: &mask,
			DstMac:     "00:00:00:00:00:00",
			DstMacMask: "00:00:00:00:00:00",
		}
		gid = int64(nosw.GROUP_TYPE_L2_FLOOD<<28) | int64(vid)<<16
		outputPort = 0xfffffffd
	}
	b := nosw.BridgingFlowEntry{
		Match:      &m,
		GroupId:    gid,
		OutputPort: outputPort,
	}
	d := nosw.FlowData{
		Bridging: &b,
	}
	return &nosw.FlowEntry{
		TableId: nosw.TABLE_ID_BRIDGING,
		Data:    &d,
	}
}

//func NewHostRegistrationEntry(vid uint16, mac net.HardwareAddr) *nosw.FlowEntry {
//	//	v := int32(0x1000 | vid)
//	//	v := int32(vid)
//	inPort := int64(1)
//	inPortMask := int64(0xffff0000)
//	//	etype := int32(0x800)
//	dstmac := mac.String()
//	m := nosw.PolicyACLFlowMatch{
//		InPort:     &inPort,
//		InPortMask: &inPortMask,
//		DstMac:     &dstmac,
//		//		EtherType: &etype,
//		//		VlanId:    &v,
//	}
//	p := nosw.PolicyACLFlowEntry{
//		Match:        &m,
//		OutputPort:   0xfffffffd,
//		ClearActions: true,
//	}
//	d := nosw.FlowData{
//		Policy: &p,
//	}
//	return &nosw.FlowEntry{
//		TableId: nosw.TABLE_ID_ACL_POLICY,
//		Data:    &d,
//	}
//}
func NewTerminationMacEntry(vid uint16, mac net.HardwareAddr) *nosw.FlowEntry {
	m := nosw.TerminationMacFlowMatch{
		VlanId: int32(vid),
		DstMac: mac.String(),
	}
	t := nosw.TerminationMacFlowEntry{
		Match: &m,
	}
	d := nosw.FlowData{
		Termination: &t,
	}
	return &nosw.FlowEntry{
		TableId: nosw.TABLE_ID_TERMINATION_MAC,
		Data:    &d,
	}
}

func NewHostRegistrationEntry(vid uint16, ip net.IP) *nosw.FlowEntry {
	//	v := int32(0x1000 | vid)
	//	v := int32(vid)
	inPort := int64(1)
	inPortMask := int64(0xffff0000)
	etype := int32(0x800)
	dstip := ip.String()
	m := nosw.PolicyACLFlowMatch{
		InPort:     &inPort,
		InPortMask: &inPortMask,
		DstIp4:     &dstip,
		EtherType:  &etype,
	}
	p := nosw.PolicyACLFlowEntry{
		Match:        &m,
		OutputPort:   0xfffffffd,
		ClearActions: true,
	}
	d := nosw.FlowData{
		Policy: &p,
	}
	return &nosw.FlowEntry{
		TableId: nosw.TABLE_ID_ACL_POLICY,
		Data:    &d,
	}
}

func NewVLANFlowEntry(vid uint16, port uint32, tagged bool) *nosw.FlowEntry {
	var e nosw.VlanFlowEntry
	if tagged == true {
		v := int32(vid) | 0x1000
		mask := int32(0x1fff)
		log.Debugf("vid/mask: %x/%x", v, mask)

		m := nosw.VlanFlowMatch{
			InPort:     int64(port),
			VlanId:     &v,
			VlanIdMask: &mask,
		}
		e = nosw.VlanFlowEntry{
			Match: &m,
		}
	} else {
		v := int32(0)
		mask := int32(0x0fff)
		log.Debugf("vid/mask: %x/%x", v, mask)
		m := nosw.VlanFlowMatch{
			InPort:     int64(port),
			VlanId:     &v,
			VlanIdMask: &mask,
		}
		nv := int32(vid)
		e = nosw.VlanFlowEntry{
			Match:      &m,
			NewVlanId_: &nv,
		}
	}
	d := nosw.FlowData{
		Vlan: &e,
	}
	return &nosw.FlowEntry{
		TableId: nosw.TABLE_ID_VLAN,
		Data:    &d,
	}
}

func NewL3UnicastEntry(ipaddr net.IP, prefixlen int, gid uint32) *nosw.FlowEntry {
	mask := net.CIDRMask(prefixlen, 32)
	ipv4mask := net.IPv4(mask[0], mask[1], mask[2], mask[3])

	m := nosw.UnicastRoutingFlowMatch{
		DstIp4:     ipaddr.String(),
		DstIp4Mask: ipv4mask.String(),
	}
	e := nosw.UnicastRoutingFlowEntry{
		Match:   &m,
		GroupId: int64(gid),
	}
	d := nosw.FlowData{
		Unicast: &e,
	}
	return &nosw.FlowEntry{
		TableId: nosw.TABLE_ID_UNICAST_ROUTING,
		Data:    &d,
	}
}
func NewAddFlowRequest(e *nosw.FlowEntry) *Request {
	resCh := make(chan *Response, 1)
	return &Request{
		Event: ADD_FLOW,
		Data:  e,
		ResCh: resCh,
	}
}

func (r *Requester) AddFlow(flow *nosw.FlowEntry) error {
	req := NewAddFlowRequest(flow)
	r.ReqCh <- req
	res := <-req.ResCh
	if res.Err != nil {
		return res.Err
	}
	return nil
}

func (r *Requester) AddL2Flow(vid uint16, port uint32, mac net.HardwareAddr) error {
	return r.AddFlow(NewL2FlowEntry(vid, port, mac, false))
}

func (r *Requester) AddL2FloodFlow(vid uint16) error {
	return r.AddFlow(NewL2FlowEntry(vid, 0, nil, true))
}

func (r *Requester) AddVLANFlow(vid uint16, port uint32, tagged bool) error {
	return r.AddFlow(NewVLANFlowEntry(vid, port, tagged))
}

func (r *Requester) AddHostRegistrationFlow(vid uint16, ip net.IP) error {
	return r.AddFlow(NewHostRegistrationEntry(vid, ip))
}

func (r *Requester) AddTerminationMacFlow(vid uint16, mac net.HardwareAddr) error {
	return r.AddFlow(NewTerminationMacEntry(vid, mac))
}

func (r *Requester) AddL3UnicastFlow(ipaddr net.IP, mask int, gid uint32) error {
	return r.AddFlow(NewL3UnicastEntry(ipaddr, mask, gid))
}

func (r *Requester) DeleteL3UnicastFlow(ipaddr net.IP, mask int, gid uint32) error {
	return r.DeleteFlow(NewL3UnicastEntry(ipaddr, mask, gid))
}

func NewDeleteFlowRequest(e *nosw.FlowEntry) *Request {
	resCh := make(chan *Response, 1)
	return &Request{
		Event: DEL_FLOW,
		Data:  e,
		ResCh: resCh,
	}
}

func (r *Requester) DeleteFlow(flow *nosw.FlowEntry) error {
	req := NewDeleteFlowRequest(flow)
	r.ReqCh <- req
	res := <-req.ResCh
	if res.Err != nil {
		return res.Err
	}
	return nil
}

func (r *Requester) GetGroups() ([]*nosw.GroupEntry, error) {
	resCh := make(chan *Response, 1)
	req := &Request{
		Event: GET_GROUPS,
		ResCh: resCh,
	}
	r.ReqCh <- req
	res := <-req.ResCh
	if res.Err != nil {
		return nil, res.Err
	}
	return res.Data.([]*nosw.GroupEntry), nil

}

func (r *Requester) AddGroup(group *nosw.GroupEntry) error {
	resCh := make(chan *Response, 1)
	req := &Request{
		Event: ADD_GROUP,
		Data:  group,
		ResCh: resCh,
	}
	r.ReqCh <- req
	res := <-req.ResCh
	if res.Err != nil {
		return res.Err
	}
	return nil
}

func (r *Requester) AddL2FloodGroup(vid uint16) (int64, error) {
	gid := int64(nosw.GROUP_TYPE_L2_FLOOD<<28) | int64(vid)<<16 | 0
	return gid, r.AddGroup(&nosw.GroupEntry{gid})

}

func (r *Requester) AddL2InterfaceGroup(vid uint16, port uint32) (int64, error) {
	gid := int64(nosw.GROUP_TYPE_L2_INTERFACE<<28) | int64(vid)<<16 | int64(port)
	return gid, r.AddGroup(&nosw.GroupEntry{gid})
}

func (r *Requester) AddL3UnicastGroup(vid uint16, port uint32, src net.HardwareAddr, dst net.HardwareAddr) (uint32, error) {
	gid := int64(nosw.GROUP_TYPE_L3_UNICAST<<28) | r.nextUnicastGroupIndex
	l2_gid := int64(nosw.GROUP_TYPE_L2_INTERFACE<<28) | int64(vid)<<16 | int64(port)
	r.nextUnicastGroupIndex += 1
	err := r.AddGroup(&nosw.GroupEntry{gid})
	if err != nil {
		return 0, err
	}
	b := &nosw.L3UnicastGroupBucketData{
		SrcMac: src.String(),
		DstMac: dst.String(),
		VlanId: int32(vid),
	}
	d := &nosw.BucketData{
		L3Unicast: b,
	}
	e := &nosw.GroupBucketEntry{
		GroupId:          gid,
		BucketIndex:      0,
		Bucket:           d,
		ReferenceGroupId: l2_gid,
	}
	return uint32(gid), r.AddBucket(e)
}

func (r *Requester) DeleteGroup(group *nosw.GroupEntry) error {
	resCh := make(chan *Response, 1)
	req := &Request{
		Event: DEL_GROUP,
		Data:  group,
		ResCh: resCh,
	}
	r.ReqCh <- req
	res := <-req.ResCh
	if res.Err != nil {
		return res.Err
	}
	return nil
}
func (r *Requester) GetBuckets(group *nosw.GroupEntry) ([]*nosw.GroupBucketEntry, error) {
	resCh := make(chan *Response, 1)
	req := &Request{
		Event: GET_BUCKETS,
		Data:  group,
		ResCh: resCh,
	}
	r.ReqCh <- req
	res := <-req.ResCh
	if res.Err != nil {
		return nil, res.Err
	}
	return res.Data.([]*nosw.GroupBucketEntry), nil
}

func (r *Requester) AddBucket(bucket *nosw.GroupBucketEntry) error {
	resCh := make(chan *Response, 1)
	req := &Request{
		Event: ADD_BUCKET,
		Data:  bucket,
		ResCh: resCh,
	}
	r.ReqCh <- req
	res := <-req.ResCh
	if res.Err != nil {
		return res.Err
	}
	return nil
}

func (r *Requester) AddL2FloodBucket(fgid int64, gid int64, index int) error {
	index64 := int64(index)
	e := &nosw.GroupBucketEntry{
		GroupId:          fgid,
		BucketIndex:      index64,
		ReferenceGroupId: gid,
	}
	return r.AddBucket(e)

}

func (r *Requester) AddL2InterfaceBucket(gid int64, port uint32, pop bool) error {
	p := int64(port)
	var pop_ int64
	if pop == true {
		pop_ = 1
	} else {
		pop_ = 0
	}
	b := &nosw.L2InterfaceGroupBucketData{
		OutputPort: p,
		PopVlanTag: pop_,
	}
	d := &nosw.BucketData{
		L2Interface: b,
	}
	e := &nosw.GroupBucketEntry{
		GroupId:     gid,
		BucketIndex: 0,
		Bucket:      d,
	}
	return r.AddBucket(e)

}

func (r *Requester) DeleteBucket(bucket *nosw.GroupBucketEntry) error {
	resCh := make(chan *Response, 1)
	req := &Request{
		Event: DEL_BUCKET,
		Data:  bucket,
		ResCh: resCh,
	}
	r.ReqCh <- req
	res := <-req.ResCh
	if res.Err != nil {
		return res.Err
	}
	return nil
}
