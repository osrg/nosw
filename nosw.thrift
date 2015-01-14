namespace go nosw
namespace py nosw

enum ERROR
{
    NONE = 0,
    RPC = -20,
    INTERNAL = -21,
    PARAM = -22,
    ERROR = -23,
    FULL = -24,
    EXISTS = -25,
    TIMEOUT = -26,
    FAIL = -27,
    DISABLED = -28,
    UNAVAIL = -29,
    NOT_FOUND = -30,
    EMPTY = -31
}

enum TABLE_ID
{
    INGRESS_PORT = 0,
    VLAN = 10,
    TERMINATION_MAC = 20,
    UNICAST_ROUTING = 30,
    MULTICAST_ROUTING = 40,
    BRIDGING = 50,
    ACL_POLICY = 60,
}

enum GROUP_TYPE
{
    L2_INTERFACE = 0,
    L2_REWRITE = 1,
    L3_UNICAST = 2,
    L2_MULTICAST = 3,
    L2_FLOOD = 4,
    L3_INTERFACE = 5,
    L3_MULTICAST = 6,
    L3_ECMP = 7,
    L2_OVERLAY = 8,
}

exception Xception {
    1: required ERROR errorCode,
    2: optional string message
}

struct IngressPortFlowMatch {
    1: required i64 inPort,
    2: required i64 inPortMask,
    3: required i32 ethType,
    4: required i32 ethTypeMask
}

struct IngressPortFlowEntry {
    1: IngressPortFlowMatch match,
    2: TABLE_ID gotoTableId,
    3: i32 vrfAction,
    4: i32 vrf
}

struct VlanFlowMatch {
    1: required i64 inPort,
    2: optional i32 vlanId,
    3: optional i32 vlanIdMask,
}

struct VlanFlowEntry {
    1:  required VlanFlowMatch match,
    2:  optional TABLE_ID gotoTableId,
    3:  optional i32 setVlanIdAction,
    4:  optional i32 newVlanId,
}

struct TerminationMacFlowMatch {
    1: required i32 vlanId,
    2: required string dstMac = "00:00:00:00:00:00",
    3: optional i64 inPort = 0
}

struct TerminationMacFlowEntry {
    1: required TerminationMacFlowMatch match,
}

struct BridgingFlowMatch {
    1: required i32 vlanId,
    2: optional i32 vlanIdMask,
    3: optional string dstMac = "00:00:00:00:00:00",
    4: optional string dstMacMask = "ff:ff:ff:ff:ff:ff"
}

struct BridgingFlowEntry {
    1: required BridgingFlowMatch match,
    2: optional TABLE_ID gotoTableId,
    3: required i64 groupId,
    4: optional i64 outputPort = 0
}

struct UnicastRoutingFlowMatch {
    1: required string dstIp4,
    2: required string dstIp4Mask
}

struct UnicastRoutingFlowEntry {
    1: required UnicastRoutingFlowMatch match,
    2: required i64 groupId,
}

struct PolicyACLFlowMatch {
    1: optional i64 inPort,
    2: optional i64 inPortMask,
    3: optional string dstMac,
    4: optional string dstMacMask,
    5: optional i32 etherType,
    6: optional i32 etherTypeMask,
    7: optional i32 vlanId,
    8: optional i32 vlanIdMask,
    9: optional string dstIp4,
    10: optional string dstIp4Mask,
}

struct PolicyACLFlowEntry {
    1: required PolicyACLFlowMatch match,
    2: optional i64 groupId,
    3: optional i64 outputPort = 0,
    4: optional bool clearActions = 0,
}

struct FlowData {
    1: optional VlanFlowEntry vlan,
    2: optional BridgingFlowEntry bridging,
    3: optional PolicyACLFlowEntry policy,
    4: optional TerminationMacFlowEntry termination,
    5: optional UnicastRoutingFlowEntry unicast,
}

struct FlowEntry {
    1: required TABLE_ID tableId,
    2: optional i64 priority = 0,
    3: required FlowData data,
    4: optional i64 hard_time = 0,
    5: optional i64 idle_time = 0,
    6: optional i64 cookie = 0
}

struct FlowEntryStats {
    1: i64 duration,
    2: i64 receivedPackets,
    3: i64 receivedBytes
}

struct FlowTableInfo {
    1: i64 numEntries,
    2: i64 maxEntries
}

struct GroupEntry {
    1: i64 groupId
}

struct GroupEntryStats {
    1: i64 refCount,
    2: i64 duration,
    3: i64 bucketCount
}

struct L2InterfaceGroupBucketData {
    1:  required i64 outputPort,
    2:  required i64 popVlanTag,
}

struct L3UnicastGroupBucketData {
    1: required string srcMac,
    2: required string dstMac,
    3: required i32 vlanId
}

struct BucketData {
    1: optional L2InterfaceGroupBucketData l2Interface
    2: optional L3UnicastGroupBucketData l3Unicast
}

struct GroupBucketEntry {
    1: required i64 groupId,
    2: required i64 bucketIndex,
    3: optional i64 referenceGroupId = 0,
    4: optional BucketData bucket
}

struct GroupTableInfo {
    1: required i64 numGroupEntries,
    2: required i64 maxGroupEntries,
    3: required i64 maxBucketEntries
}

struct PortFeature {
    1: required i32 curr,
    2: required i32 advertised,
    3: required i32 supported,
    4: required i32 peer
}

struct PortStats {
    1:  required i64 rx_packets,
    2:  required i64 tx_packets,
    3:  required i64 rx_bytes,
    4:  required i64 tx_bytes,
    5:  required i64 rx_errors,
    6:  required i64 tx_errors,
    7:  required i64 rx_drops,
    8:  required i64 tx_drops,
    9:  required i64 rx_frame_err,
    10: required i64 rx_over_err,
    11: required i64 rx_crc_err,
    12: required i64 collisions,
    13: required i64 duration_seconds
}

struct Packet {
    1: required binary data
    2: optional i64 reason,
    3: optional TABLE_ID tableId,
    4: optional i64 inPortNum,
    5: optional i64 outPortNum,
}

service Escalator {
# Pkt APIs
    void tossPacket(1:Packet pkt) throws (1:Xception err)
}

service Nosw extends Escalator {
# Flow Table APIs
    bool isSupported(1:TABLE_ID tableId) throws (1:Xception err),
    FlowTableInfo getFlowTableInfo(1:TABLE_ID tableId) throws (1:Xception err),
    void addFlow(1:FlowEntry entry) throws (1:Xception err),
    void modifyFlow(1:FlowEntry entry) throws (1:Xception err),
    void deleteFlow(1:FlowEntry entry) throws (1:Xception err),
    list<FlowEntry> getFlows(1:TABLE_ID tableId) throws (1:Xception err),
    FlowEntry getNextFlow(1:FlowEntry entry) throws (1:Xception err),
    FlowEntryStats getFlowStats(1:FlowEntry entry) throws (1:Xception err),
    FlowEntry getFlowByCookie(1:i64 cookie) throws (1:Xception err),
    void deleteFlowByCookie(1:i64 cookie) throws (1:Xception err),
# Group Table APIs
    GroupTableInfo getGroupTableInfo(1:i16 type) throws (1:Xception err),
    void addGroup(1:GroupEntry entry) throws (1:Xception err),
    void deleteGroup(1:GroupEntry entry) throws (1:Xception err),
    list<GroupEntry> getGroups() throws (1:Xception err),
    GroupEntryStats getGroupStats(1:GroupEntry entry) throws (1:Xception err),
    void addBucket(1:GroupBucketEntry entry) throws (1:Xception err),
    void deleteBucket(1:GroupBucketEntry entry) throws (1:Xception err),
    void deleteAllBucket(1:GroupBucketEntry entry) throws (1:Xception err),
    void modifyBucket(1:GroupBucketEntry entry) throws (1:Xception err),
    list<GroupBucketEntry> getBuckets(1:GroupEntry entry) throws (1:Xception err),
# Port APIs
    PortStats getPortStats(1:i32 portNum) throws (1:Xception err),
    void clearPortStats(1:i32 portNum) throws (1:Xception err)
}
