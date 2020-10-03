package server

import (
	"bufio"
	"container/list"
	"crypto/tls"
	"encoding/json"
	"github.com/xtaci/smux"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
	"u2ps/common"
	"u2ps/utils"
)

var (
	ri                   = 0
	conn                 net.Conn
	listen               net.Listener
	Node                 common.Node
	TunnelMap            = make(map[int64]common.Tunnel)
	ClientDataStreamMap  = ClientDataStream{Data: make(map[int64]*list.List)}
	ClientControlConnMap = make(map[int64]net.Conn)
	ListeningMap         = make(map[int64]interface{})
	ListeningUdpMap      = make(map[int64]*net.UDPConn)
	UdpMap               = make(map[string]*smux.Stream)
)

func listenClient() {
	//监听
	var err error
	listen, err = net.Listen("tcp", "0.0.0.0:"+strconv.Itoa(Node.Port))
	if err != nil {
		return
	}
	for {
		//处理客户端连接
		conn, err := listen.Accept()
		if err != nil {
			break
		}
		go clientConn(conn)
	}
}

//客户端连接
func clientConn(conn net.Conn) {
	var clientId int64
	msg, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		log.Printf("接收客户端端数据出错")
	} else {
		msg = strings.Replace(msg, "\n", "", -1)
		b := []byte(msg)
		m := common.Msg{}
		err := json.Unmarshal(b, &m)
		if err == nil {
			switch m.Type {
			//客户端控制连接消息
			case string(common.ClientConnNodeControl):
				if m.Data != nil {
					common.ToStruct(&clientId, m.Data)
					ClientControlConnMap[clientId] = conn
					go clientConnTask(conn, clientId)
				}
			case string(common.ClientConnNodeData):
				common.ToStruct(&clientId, m.Data)
				if ClientDataStreamMap.Get(clientId) != nil {
					ClientDataStreamMap.delete(clientId)
				}
				session, err := smux.Server(conn, nil)
				if err == nil {
					go SessionListen(session, clientId)
				}

			default:
				log.Println(msg)
			}
		}
	}
}

type ClientDataStream struct {
	Data map[int64]*list.List
	Lock sync.Mutex
}

func (d ClientDataStream) Get(k int64) *list.List {
	d.Lock.Lock()
	defer d.Lock.Unlock()
	return d.Data[k]
}
func (d ClientDataStream) Set(k int64, v *list.List) {
	d.Lock.Lock()
	defer d.Lock.Unlock()
	d.Data[k] = v
}
func (d ClientDataStream) HasKey(k int64) (has bool, v *list.List) {
	d.Lock.Lock()
	defer d.Lock.Unlock()
	if l, ok := d.Data[k]; ok {
		return true, l
	}
	return false, nil
}
func (d ClientDataStream) delete(k int64) {
	d.Lock.Lock()
	defer d.Lock.Unlock()
	delete(d.Data, k)
}
func SessionListen(session *smux.Session, id int64) {
	defer func() {
		ClientDataStreamMap.delete(id)
		session.Close()
	}()
	var nodeDataConnId string
	for {
		stream, err := session.AcceptStream()
		if err == nil {
			go func() {
				msg, err := bufio.NewReader(stream).ReadString('\n')
				if err != nil {
					log.Printf("接收客户端端数据出错")
					return
				} else {
					msg = strings.Replace(msg, "\n", "", -1)
					b := []byte(msg)
					m := common.Msg{}
					err := json.Unmarshal(b, &m)
					if err == nil {
						switch m.Type {
						case string(common.ClientConnNodeData):
							if m.Data != nil {
								common.ToStruct(&nodeDataConnId, m.Data)
								dataConn := common.NodeDataConn{Stream: stream, Id: nodeDataConnId}
								if has, l := ClientDataStreamMap.HasKey(id); has {
									l.PushBack(dataConn)
									//存在
								} else {
									l := list.New()
									l.PushBack(dataConn)
									ClientDataStreamMap.Set(id, l)
								}
							}
						default:
							log.Println(msg)
						}
					}
				}
			}()
		} else {
			break
		}
	}
}

//客户端消息处理
func clientConnTask(conn net.Conn, clientId int64) {
	defer func() {
		conn.Close()
		delete(ClientControlConnMap, clientId)
	}()
	for {
		msg, err := bufio.NewReader(conn).ReadString('\n')
		if err != nil {
			return
		} else {
			msg = strings.Replace(msg, "\n", "", -1)
			b := []byte(msg)
			m := common.Msg{}
			err := json.Unmarshal(b, &m)
			if err == nil {
				switch m.Type {
				case string(common.NewTunnelConn):
					nc := common.NewConn{}
					common.ToStruct(&nc, m.Data)
				case string(common.Heartbeat):
					//接收到心跳正常.
					common.SendStructType(conn, common.Heartbeat)
				default:
					log.Println(msg)
				}
				if m.Msg != "" {
					log.Printf(m.Msg)
				}
			}
		}
	}
}
func GetControlClientConn(id int64) net.Conn {
	return ClientControlConnMap[id]
}
func Conn() {
	if common.Token == "" {
		log.Printf("请加上token参数 -t")
		os.Exit(1)
	}
	for ri < common.MaxRi {
		var err error
		conn, err = net.Dial("tcp", common.HostInfo)
		if err != nil {
			log.Printf("连接" + common.HostInfo + "失败")
		} else {
			ri = 0
			log.Printf("开始验证Token")
			//认证
			authentication()
			doTask()
		}
		time.Sleep(time.Second * 10)
		ri++
		log.Printf("连接已断开..重试第" + strconv.Itoa(ri) + "次")
	}
	log.Printf("停止尝试,退出服务")
}
func doTask() {
	for {
		msg, err := bufio.NewReader(conn).ReadString('\n')
		if err != nil {
			log.Printf("接收U2PS服务端数据出错")
			break
		} else {
			msg = strings.Replace(msg, "\n", "", -1)
			b := []byte(msg)
			m := common.Msg{}
			err := json.Unmarshal(b, &m)
			if err == nil {
				switch m.Type {
				//连接验证错误
				case string(common.AuthenticationResultErr):
					log.Printf(m.Msg)
					conn.Close()
					os.Exit(1)
				case string(common.TypeAuthenticationResultOk):
					//node连接会返回端口
					if m.Data != nil && listen == nil {
						rs := struct {
							Node     common.Node `json:"node"`
							Versions string      `json:"versions"`
						}{}
						common.ToStruct(&rs, m.Data)
						if rs.Versions != common.Versions {
							log.Printf("当前版本不是最新版本,请到官网下载最新版本! https://u2ps.com")
							conn.Close()
							os.Exit(1)
						}
						Node = rs.Node
						go common.SendStruct(conn, common.Heartbeat, "Node", Node.IP)
						if utils.ScanPortOne(Node.Port) {
							log.Println("端口被占用:" + strconv.Itoa(Node.Port))
							conn.Close()
							os.Exit(1)
						}
						go listenClient()
						go getAllTunnel()
					}
				case string(common.AllTunnel):
					var ts []common.Tunnel
					common.ToStruct(&ts, m.Data)
					go addTunnel(ts)
				case string(common.IsPortUse):
					var vPort int
					common.ToStruct(&vPort, m.Data)
					if utils.ScanPortOne(vPort) {
						common.SendStruct(conn, common.IsPortUse, m.Msg, true)
					} else {
						common.SendStruct(conn, common.IsPortUse, m.Msg, false)
					}
					continue
				case string(common.UpdateTunnel):
					var tunnel common.Tunnel
					common.ToStruct(&tunnel, m.Data)
					if ListeningMap[tunnel.ID] != nil {
						//关掉旧的
						switch TunnelMap[tunnel.ID].Type {
						case 1:
							ListeningMap[tunnel.ID].(net.Listener).Close()
						case 2:
							ListeningUdpMap[tunnel.ID].Close()
						case 3:
							ListeningMap[tunnel.ID].(*http.Server).Close()
						case 4:
							ListeningMap[tunnel.ID].(net.Listener).Close()
							ListeningUdpMap[tunnel.ID].Close()
						}
						delete(ListeningMap, tunnel.ID)
					}
					//添加新的隧道
					tunnels := make([]common.Tunnel, 1)
					tunnels[0] = tunnel
					go addTunnel(tunnels)
				case string(common.DeleteTunnel):
					var tunnelId int64
					common.ToStruct(&tunnelId, m.Data)
					if ListeningMap[tunnelId] != nil {
						//关掉旧的
						switch TunnelMap[tunnelId].Type {
						case 1:
							ListeningMap[tunnelId].(net.Listener).Close()
						case 2:
							ListeningUdpMap[tunnelId].Close()
						case 3:
							ListeningMap[tunnelId].(*http.Server).Close()
						case 4:
							ListeningMap[tunnelId].(net.Listener).Close()
							ListeningUdpMap[tunnelId].Close()
						}
					}
					delete(TunnelMap, tunnelId)
					delete(ListeningUdpMap, tunnelId)
				case string(common.DeleteNode):
					log.Println("收到节点删除信息,退出node")
					os.Exit(1)
				case string(common.UpdateNode):
					var node common.Node
					common.ToStruct(&node, m.Data)
					if node.Port != Node.Port {
						listen.Close()
						Node = node
						for _, c := range ClientControlConnMap {
							c.Close()
						}
						go listenClient()
					}
				case string(common.Heartbeat):
					//接收到心跳正常.
					go common.SendStruct(conn, common.Heartbeat, "Node", Node.IP)
				default:
					log.Println(msg)
				}
				if m.Msg != "" {
					log.Printf(m.Msg)
				}
			}
		}
	}
}

//认证
func authentication() {
	common.SendStructTypeAndData(conn, common.AuthenticationServer, common.Token)
}

//获取所有隧道
func getAllTunnel() {
	common.SendStructType(conn, common.AllTunnel)
}

//添加隧道
func addTunnel(ts []common.Tunnel) {
	for _, t := range ts {
		TunnelMap[t.ID] = t
		if t.Open {
			//建立隧道监听
			if utils.ScanPortOne(t.ServicePort) {
				log.Println("隧道端口被占用:" + strconv.Itoa(t.ServicePort))
			} else {
				switch t.Type {
				case 1:
					go NewTunnelTcp(t).listenTcp()
					go CheckWeb(t)
				case 2:
					go NewTunnelUdp(t).listenUdp()
				case 3:
					go NewTunnelHttp(t).listenHttp()
				case 4:
					go NewTunnelTcp(t).listenTcp()
					go NewTunnelUdp(t).listenUdp()
				}
			}
		}
	}
}

type TunnelTcp struct {
	tunnelId int64
	s        net.Listener
	tunnel   common.Tunnel
}
type TunnelUdp struct {
	tunnelId int64
	s        *net.UDPConn
	tunnel   common.Tunnel
}
type TunnelHttp struct {
	tunnelId int64
	s        http.Server
	tunnel   common.Tunnel
}

func NewTunnelHttp(tunnel common.Tunnel) *TunnelHttp {
	return &TunnelHttp{tunnel: tunnel, tunnelId: tunnel.ID}
}
func NewTunnelTcp(tunnel common.Tunnel) *TunnelTcp {
	return &TunnelTcp{tunnel: tunnel, tunnelId: tunnel.ID}
}
func (t *TunnelHttp) listenHttp() {
	t.s = http.Server{
		Addr:         "0.0.0.0:" + strconv.Itoa(t.tunnel.ServicePort),
		Handler:      http.HandlerFunc(t.handleNormal),
		TLSNextProto: make(map[string]func(*http.Server, *tls.Conn, http.Handler)),
	}
	err := t.s.ListenAndServe()
	if err != nil {
		log.Println(err)
	}
	ListeningMap[t.tunnelId] = &t.s
	delete(ListeningMap, t.tunnelId)
}
func (t *TunnelHttp) handleNormal(w http.ResponseWriter, r *http.Request) {
	cid := t.tunnel.ClientID
	clientControlConn := GetControlClientConn(cid)
	if clientControlConn == nil {
		return
	}
	dataConn := getDataStream(cid)
	if dataConn.Stream == nil {
		return
	}
	request, err2 := httputil.DumpRequest(r, true)
	if err2 != nil {
		return
	}
	dataConn.Stream.Write(request)
	//向客户端发送消息建立一个通向代理端口的连接 id为connId
	common.SendStruct(clientControlConn, common.NewTunnelConn, "", common.NewConn{ConnUid: dataConn.Id, TunnelId: t.tunnelId})
	//类型转换
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "Hijacking not supported", http.StatusInternalServerError)
		return
	}
	//接管连接
	clientConn, _, err := hijacker.Hijack()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
	}
	go TcpBridge(clientConn, dataConn.Stream, t.tunnelId, t.tunnel.NodeID)
}
func (t *TunnelTcp) listenTcp() {
	var err error
	t.s, err = net.Listen("tcp", "0.0.0.0:"+strconv.Itoa(t.tunnel.ServicePort))
	if err != nil {
		return
	}
	ListeningMap[t.tunnelId] = t.s
	for {
		//处理客户端连接
		conn, err := t.s.Accept()
		if err != nil {
			break
		}
		go t.handleNormal(conn)
	}
	delete(ListeningMap, t.tunnelId)
}
func (t *TunnelTcp) handleNormal(conn net.Conn) {
	cid := t.tunnel.ClientID
	clientControlConn := GetControlClientConn(cid)
	if clientControlConn == nil {
		return
	}
	dataConn := getDataStream(cid)
	//向客户端发送消息建立一个通向代理端口的连接 id为connId
	common.SendStruct(clientControlConn, common.NewTunnelConn, "", common.NewConn{ConnUid: dataConn.Id, TunnelId: t.tunnelId})
	go TcpBridge(conn, dataConn.Stream, t.tunnelId, t.tunnel.NodeID)
}

//web检查
func CheckWeb(tunnel common.Tunnel) {
	defer func() {
		if err := recover(); err != nil {
			log.Println("Panic info is: ", err)
		}
	}()
	cid := tunnel.ClientID
	for {
		_, ok := TunnelMap[tunnel.ID]
		if !ok {
			break
		}
		tunnel = TunnelMap[tunnel.ID]
		clientControlConn := GetControlClientConn(cid)
		if clientControlConn != nil {
			dataConn := getDataStream(cid)
			//向客户端发送消息建立一个通向代理端口的连接 id为connId
			common.SendStruct(clientControlConn, common.NewTunnelConn, "", common.NewConn{ConnUid: dataConn.Id, TunnelId: tunnel.ID})
			dataConn.Stream.Write([]byte("/ \\r\\n"))
			buf := make([]byte, 1024)
			l, err := dataConn.Stream.Read(buf)
			if err == nil {
				s := strings.ToUpper(string(buf[0:l]))
				log.Println(s)
				if strings.Contains(s, "HTTP") && strings.Contains(s, "REQUEST") {
					common.SendStruct(conn, common.TcpWeb, "", tunnel.ID)
					ListeningMap[tunnel.ID].(net.Listener).Close()
					delete(ListeningMap, tunnel.ID)
					delete(TunnelMap, tunnel.ID)
				}
			}
			if dataConn.Stream != nil {
				dataConn.Stream.Close()
			}
		}
		time.Sleep(time.Second * 10)
	}
}
func TcpBridge(connP net.Conn, connC *smux.Stream, tid int64, nid int64) {
	if connP != nil && connC != nil {
		go common.CopyBufferUpdateFlow(connP, connC, conn, tid, nid, true)
		common.CopyBufferUpdateFlow(connC, connP, conn, tid, nid, false)
	}
}
func UdpBridge(connP *net.UDPConn, connC *smux.Stream, addr *net.UDPAddr, tid int64, nid int64) {
	var flow int64
	defer func() {
		if flow > 0 {
			go common.SendStruct(conn, common.UpdateFlow, "", common.FlowType{IsUp: false, TunnelId: tid, NodeId: nid, Flow: flow})
		}
		if err := recover(); err != nil {
			log.Println("Panic info is: ", err)
		}
	}()
	if connP != nil && connC != nil {
		buf := make([]byte, 65507)
		for {
			n, err := connC.Read(buf)
			if err == nil {
				l, _ := connP.WriteToUDP(buf[0:n], addr)
				if l > 0 {
					flow += int64(l)
					if flow > 1024*1024*10 {
						go common.SendStruct(conn, common.UpdateFlow, "", common.FlowType{IsUp: false, TunnelId: tid, NodeId: nid, Flow: flow})
						flow = 0
					}
				}
			} else {
				connC.Close()
				delete(UdpMap, addr.String())
				break
			}
		}
	}
}
func getDataStream(id int64) common.NodeDataConn {
	l := ClientDataStreamMap.Get(id)
	if l != nil && l.Len() > 1 {
		return l.Remove(l.Back()).(common.NodeDataConn)
	}
	return common.NodeDataConn{}
}
func NewTunnelUdp(tunnel common.Tunnel) *TunnelUdp {
	return &TunnelUdp{tunnel: tunnel, tunnelId: tunnel.ID}
}
func (t *TunnelUdp) listenUdp() {
	var flow int64
	var err error
	udpAddr, _ := net.ResolveUDPAddr("udp", "0.0.0.0:"+strconv.Itoa(t.tunnel.ServicePort))
	t.s, _ = net.ListenUDP("udp", udpAddr) //创建监听链接
	if err != nil {
		return
	}
	ListeningUdpMap[t.tunnelId] = t.s
	buf := make([]byte, 65507)
	for {
		n, addr, err := t.s.ReadFromUDP(buf)
		if err == nil {
			if UdpMap[addr.String()] == nil {
				go t.handleNormal(addr, buf[0:n])
			} else {
				l, err := UdpMap[addr.String()].Write(buf[0:n])
				if err != nil {
					delete(UdpMap, addr.String())
				}
				flow += int64(l)
				if flow > 1024*1024*10 {
					go common.SendStruct(conn, common.UpdateFlow, "", common.FlowType{IsUp: true, TunnelId: t.tunnelId, NodeId: t.tunnel.NodeID, Flow: flow})
				}
			}
		} else {
			break
		}
	}
	delete(ListeningUdpMap, t.tunnelId)
}
func (t *TunnelUdp) handleNormal(addr *net.UDPAddr, data []byte) {
	cid := t.tunnel.ClientID
	clientControlConn := GetControlClientConn(cid)
	dataConn := getDataStream(cid)
	if clientControlConn == nil || dataConn.Stream == nil {
		return
	}
	UdpMap[addr.String()] = dataConn.Stream
	dataConn.Stream.Write(data)
	//向客户端发送消息建立一个通向代理端口的连接 id为connId
	common.SendStruct(clientControlConn, common.NewTunnelConn, "", common.NewConn{ConnUid: dataConn.Id, TunnelId: t.tunnelId})
	go UdpBridge(t.s, dataConn.Stream, addr, t.tunnelId, t.tunnel.NodeID)
}
