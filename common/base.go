package common

import (
	"bytes"
	"encoding/json"
	"github.com/xtaci/smux"
	"log"
	"net"
	"time"
)

type Msg struct {
	Time int64       `json:"time"`
	Type string      `json:"type"`
	Msg  string      `json:"msg"`
	Data interface{} `json:"data"`
}

type Tunnel struct {
	ClientID     int64  `json:"clientId"`
	CreationTime int64  `json:"creationTime"`
	ID           int64  `json:"id"`
	NodeID       int64  `json:"nodeId"`
	Open       	 bool   `json:"Open"`
	ServicePort  int    `json:"servicePort"`
	TargetIP     string `json:"targetIp"`
	TargetPort   int    `json:"targetPort"`
	Type         int    `json:"type"`
	Username     string `json:"username"`
}
type Node struct {
	AllowWeb           bool    `json:"allowWeb"`
	CountriesRegions   string  `json:"countriesRegions"`
	CreationTime       int64   `json:"creationTime"`
	FlowRatio          float64 `json:"flowRatio"`
	ID                 int64   `json:"id"`
	IP                 string  `json:"ip"`
	Online             bool    `json:"online"`
	Open               bool    `json:"open"`
	Port               int     `json:"port"`
	Ports              string  `json:"ports"`
	Username           string  `json:"username"`
}
type Client struct {
	ClientIP     string `json:"clientIp"`
	CreationTime int64  `json:"creationTime"`
	ID           int64  `json:"id"`
	OnLine       bool   `json:"onLine"`
	Remark       string `json:"remark"`
	SecretKey    string `json:"secretKey"`
	Username     string `json:"username"`
}
type NewConn struct {
	ConnId   int64  `json:"conn_id"`
	TunnelId int64  `json:"tunnel_id"`
	ConnUid  string `json:"conn_uid"`
}
type NodeDataConn struct {
	Stream *smux.Stream
	Id     string
}
type FlowType struct {
	IsUp bool `json:"is_up"`
	NodeId int64 `json:"node_id"`
	TunnelId int64 `json:"tunnel_id"`
	Flow int64 `json:"flow"`
}
type MsgType string

const (
	AuthenticationClient       MsgType = "AuthenticationClient"
	AuthenticationResultErr    MsgType = "AuthenticationResultErr"
	TypeAuthenticationResultOk MsgType = "TypeAuthenticationResultOk"
	AllTunnel                  MsgType = "AllTunnel"
	AuthenticationServer       MsgType = "AuthenticationServer"
	ClientConnNodeControl      MsgType = "ClientConnNodeControl"
	ClientConnNodeData         MsgType = "ClientConnNodeData"
	NewTunnelConn              MsgType = "NewTunnelConn"
	UpdateTunnel               MsgType = "UpdateTunnel"
	IsPortUse                  MsgType = "IsPortUse"
	DeleteTunnel			   MsgType = "DeleteTunnel"
	DeleteNode				   MsgType = "DeleteNode"
	UpdateNode				   MsgType = "UpdateNode"
	UpdateFlow    			   MsgType = "UpdateFlow"
	TcpWeb					   MsgType = "TcpWeb"
	Heartbeat				   MsgType = "Heartbeat"
)

func GetMsg(Type MsgType, msg string, data interface{}) Msg {
	defer func() {
		if err := recover(); err != nil {
			log.Println("Panic info is: ", err)
		}
	}()
	return Msg{Time: time.Now().UnixNano() / 1e6, Type: string(Type), Msg: msg, Data: data}
}
func SendStructType(conn net.Conn, Type MsgType) {
	defer func() {
		if err := recover(); err != nil {
			log.Println("Panic info is: ", err)
		}
	}()
	SendStruct(conn, Type, "", nil)
}
func SendStructTypeAndData(conn net.Conn, Type MsgType, data interface{}) {
	defer func() {
		if err := recover(); err != nil {
			log.Println("Panic info is: ", err)
		}
	}()
	SendStruct(conn, Type, "", data)
}
func SendStruct(conn net.Conn, Type MsgType, msg string, data interface{}) {
	defer func() {
		if err := recover(); err != nil {
			log.Println("Panic info is: ", err)
		}
	}()
	_, err := conn.Write(BytesCombine(ToJsonBytes(GetMsg(Type, msg, ToJsonStr(data))), []byte("\n")))
	if err != nil {
		println(err)
	}
}
func StreamSendStruct(stream *smux.Stream, Type MsgType, msg string, data interface{}) {
	defer func() {
		if err := recover(); err != nil {
			log.Println("Panic info is: ", err)
		}
	}()
	_, err := stream.Write(BytesCombine(ToJsonBytes(GetMsg(Type, msg, ToJsonStr(data))), []byte("\n")))
	if err != nil {
		println(err)
	}
}
func ToJsonBytes(s interface{}) []byte {
	//Person 结构体转换为对应的 Json
	jsonBytes, err := json.Marshal(s)
	if err != nil {
		log.Println(err)
	}
	return jsonBytes
}
func BytesCombine(pBytes ...[]byte) []byte {
	return bytes.Join(pBytes, []byte(""))
}
func ToStruct(s interface{}, strJson interface{}) {
	b := []byte(strJson.(string))
	err := json.Unmarshal(b, &s)
	if err != nil {
		log.Println(err)
	}
}
func ToJsonStr(s interface{}) string {
	return string(ToJsonBytes(s))
}
func CopyBuffer(dst net.Conn, src net.Conn) int64{
	defer func() {
		dst.Close()
		src.Close()
		if err := recover(); err != nil {
			log.Println("Panic info is: ", err)
		}
	}()
	var flow int64
	buf := make([]byte, 1024*32)
	for {
		nr, er := src.Read(buf)
		if er != nil {
			break
		}
		if nr > 0 {
			nw, we := dst.Write(buf[0:nr])
			if nr != nw||we!=nil{
				break
			}
			flow+=int64(nw)
		}
	}
	return flow
}
func CopyBufferUpdateFlow(dst net.Conn, src net.Conn,conn net.Conn,tid int64,nid int64,isUp bool) {
	var flow int64
	defer func() {
		dst.Close()
		src.Close()
		if flow>0{
			SendStruct(conn,UpdateFlow,"",FlowType{IsUp: isUp,TunnelId: tid,NodeId: nid,Flow: flow})
		}
		if err := recover(); err != nil {
			log.Println("Panic info is: ", err)
		}
	}()
	buf := make([]byte, 1024*32)
	for {
		nr, er := src.Read(buf)
		if er != nil {
			break
		}
		if nr > 0 {
			nw, we := dst.Write(buf[0:nr])
			if nw>0 {
				flow+=int64(nw)
				if flow>1024*1024*10{
					SendStruct(conn,UpdateFlow,"",FlowType{IsUp: isUp,TunnelId: tid,NodeId: nid,Flow: flow})
					flow=0
				}
			}
			if nr != nw||we!=nil{
				break
			}
		}
	}
}
