package client

import (
	"errors"
	"github.com/rs/xid"
	"github.com/xtaci/smux"
	"log"
	"net"
	"strconv"
	"time"
	"u2ps/common"
)

var (
	NodeDataSessionMap = make(map[int64]*smux.Session)
	StreamMap          = make(map[string]NodeDataStream)
	NodeStreamNumMap   = make(map[int64]int64)
)

type NodeDataStream struct {
	id int64
	*smux.Stream
}
func NodeInitDataConn(id int64) {
	defer func() {
		if err := recover(); err != nil {
			log.Println("Panic info is: ", err)
		}
	}()
	node := NodeMap[id]
	conn, err := net.Dial("tcp", node.IP+":"+strconv.Itoa(node.Port))
	if err == nil {
		common.SendStruct(conn, common.ClientConnNodeData, "", Client.ID)
		session, err := smux.Client(conn, nil)
		if err == nil {
			NodeDataSessionMap[id] = session
			log.Println("与nodeData连接成功!", id)
			Add10Stream(id)
		}
	} else {
		log.Println("与nodeData连接失败!", id)
	}
}
func AddNodeDataConn(id int64) common.NodeDataConn {
	defer func() {
		if err := recover(); err != nil {
			log.Println("Panic info is: ", err)
		}
	}()
	session := NodeDataSessionMap[id]
	stream, err := session.OpenStream()
	if err != nil {
		var i = 0
		node := NodeMap[id]
		log.Println("与nodeDta连接断开:", id)
		for i < common.MaxRi {
			log.Println("尝试重新连接:", i+1, "次")
			if _, ok := NodeMap[id]; ok {
			conn, err := net.Dial("tcp", node.IP+":"+strconv.Itoa(node.Port))
			if err == nil {
				common.SendStruct(conn, common.ClientConnNodeData, "", Client.ID)
				session, err := smux.Client(conn, nil)
				if err == nil {
					NodeDataSessionMap[id] = session
					return AddNodeDataConn(id)
				}
				time.Sleep(time.Duration(i)*2*time.Second + 1)}
			}
			i++
		}
		log.Println("连接失败放弃连接!")
		return common.NodeDataConn{}
	}
	uid := xid.New().String()
	common.StreamSendStruct(stream, common.ClientConnNodeData, "", uid)
	NodeStreamNumMap[id]++
	return common.NodeDataConn{Stream: stream, Id: uid}
}
func Add10Stream(id int64) {
	for i := 0; i < 10; i++ {
		conn := AddNodeDataConn(id)
		StreamMap[conn.Id] = NodeDataStream{id,conn.Stream}
	}
}
func GetStream(uid string, nin int64) *smux.Stream {
	defer func() {
		if err := recover(); err != nil {
			log.Println("Panic info is: ", err)
		}
	}()
	stream := StreamMap[uid]
	delete(StreamMap, uid)
	if NodeStreamNumMap[nin] < 6 {
		Add10Stream(nin)
	}
	NodeStreamNumMap[nin]--
	return stream.Stream
}
func NewTunnelConn(t common.Tunnel) (net.Conn, error) {
	switch t.Type {
	case 1, 3, 4:
		return net.Dial("tcp", t.TargetIP+":"+strconv.Itoa(t.TargetPort))
	case 2:
		return net.Dial("udp", t.TargetIP+":"+strconv.Itoa(t.TargetPort))
	}
	return nil, errors.New("隧道类型有误")
}
