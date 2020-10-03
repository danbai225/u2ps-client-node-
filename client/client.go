package client

import (
	"bufio"
	"encoding/json"
	"github.com/txthinking/socks5"
	"github.com/xtaci/smux"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
	"u2ps/common"
)

var (
	ri                 = 0
	sConn              net.Conn
	Client             common.Client
	NodeControlConnMap = make(map[int64]net.Conn)
	TunnelMap          = make(map[int64]common.Tunnel)
	NodeMap            = make(map[int64]common.Node)
	Socks5Map          = make(map[int64]*socks5.Server)
)

func Conn() {
	if common.Key == "" {
		log.Printf("请加上Key参数 -k")
		os.Exit(1)
	}
	for ri < common.MaxRi {
		var err error
		sConn, err = net.Dial("tcp", common.HostInfo)
		if err != nil {
			log.Printf("连接" + common.HostInfo + "失败")
		} else {
			ri = 0
			log.Printf("开始验证Key")
			doTask()
		}
		time.Sleep(10 * time.Second)
		ri++
		log.Printf("连接已断开..重试第" + strconv.Itoa(ri) + "次")
	}
	log.Printf("停止尝试,退出服务")
}
func doTask() {
	//认证
	authentication()
	for {
		msg, err := bufio.NewReader(sConn).ReadString('\n')
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
					sConn.Close()
					os.Exit(1)
				case string(common.TypeAuthenticationResultOk):
					//客户端连接返回节点和隧道
					if m.Data != nil && len(NodeMap) == 0 {
						rs := struct {
							Tunnels  []common.Tunnel `json:"tunnels"`
							Nodes    []common.Node   `json:"nodes"`
							Client   common.Client   `json:"client"`
							Versions string          `json:"versions"`
						}{}
						common.ToStruct(&rs, m.Data)
						if rs.Versions != common.Versions {
							log.Printf("当前版本不是最新版本,请到官网下载最新版本! https://u2ps.com")
							sConn.Close()
							os.Exit(1)
						}
						Client = rs.Client
						addNodeConn(rs.Nodes)
						for _, t := range rs.Tunnels {
							TunnelMap[t.ID] = t
							if t.Type == 4 {
								go Socks5(t)
							}
						}
						go common.SendStruct(sConn, common.Heartbeat, "Client", Client.ID)
					}
				case string(common.UpdateTunnel):
					var tunnel common.Tunnel
					common.ToStruct(&tunnel, m.Data)
					TunnelMap[tunnel.ID] = tunnel
					if tunnel.Type == 4 {
						if Socks5Map[tunnel.ID] != nil {
							Socks5Map[tunnel.ID].Shutdown()
						}
						go Socks5(tunnel)
					}
				case string(common.DeleteTunnel):
					var tunnelId int64
					common.ToStruct(&tunnelId, m.Data)
					nodeId := TunnelMap[tunnelId].NodeID
					if TunnelMap[tunnelId].Type == 4 {
						if Socks5Map[tunnelId] != nil {
							Socks5Map[tunnelId].Shutdown()
						}
						delete(Socks5Map, tunnelId)
					}
					delete(TunnelMap, tunnelId)
					var flag = true
					for _, t := range TunnelMap {
						if t.NodeID == nodeId {
							flag = false
						}
					}
					if flag {
						if NodeDataSessionMap[nodeId] != nil {
							NodeDataSessionMap[nodeId].Close()
							delete(NodeDataSessionMap, nodeId)
						}
						if NodeControlConnMap[nodeId] != nil {
							NodeControlConnMap[nodeId].Close()
							delete(NodeControlConnMap, nodeId)
						}
					}
				case string(common.UpdateNode):
					var node common.Node
					common.ToStruct(&node, m.Data)
					delete(NodeMap, node.ID)
					nodes := make([]common.Node, 1)
					nodes[0] = node
					go addNodeConn(nodes)
				case string(common.Heartbeat):
					//接收到心跳正常.
					go common.SendStruct(sConn, common.Heartbeat, "Client", Client.ID)

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
func Socks5(tunnel common.Tunnel) {
	s, err := socks5.NewClassicServer("127.0.0.1:"+strconv.Itoa(tunnel.TargetPort), "127.0.0.1", "", "", 0, 60)
	if err != nil {
		log.Println(err)
		return
	}
	err = s.ListenAndServe(nil)
	if err != nil {
		log.Println(err)
		return
	}
	Socks5Map[tunnel.ID] = s
}

//认证
func authentication() {
	common.SendStructTypeAndData(sConn, common.AuthenticationClient, common.Key)
}

func addNodeConn(ns []common.Node) {
	//遍历所有节点,建立连接添加到map
	for _, n := range ns {
		NodeMap[n.ID] = n
		NConn, err := net.Dial("tcp", n.IP+":"+strconv.Itoa(n.Port))
		if err == nil {
			NodeControlConnMap[n.ID] = NConn
			go NodeInitDataConn(n.ID)
			go ControlNodeTask(NConn, n.ID)
		} else {
			log.Println("连接node失败", n.ID)
		}
	}
}

func ControlNodeTask(conn net.Conn, id int64) {
	common.SendStruct(conn, common.ClientConnNodeControl, "", Client.ID)
	go common.SendStructType(conn, common.Heartbeat)
	var ControlRi = 0
	for {
		msg, err := bufio.NewReader(conn).ReadString('\n')
		if err != nil {
			conn = nil
			log.Println("控制连接断开,准备重连:", id)
			for ControlRi < common.MaxRi {
				if _, ok := NodeMap[id]; ok {
					conn, err = net.Dial("tcp", NodeMap[id].IP+":"+strconv.Itoa(NodeMap[id].Port))
					if err == nil {
						common.SendStruct(conn, common.ClientConnNodeControl, "", Client.ID)
						ControlRi = 0
						NodeControlConnMap[id] = conn
						go NodeInitDataConn(id)
						log.Println("重连成功", id)
						break
					}
					time.Sleep(time.Duration(ControlRi)*2*time.Second + 1)
					ControlRi++
					log.Printf("节点连接已断开..重试第" + strconv.Itoa(ControlRi) + "次")
				} else {
					return
				}
			}
			if ControlRi >= common.MaxRi {
				log.Printf("停止尝试")
				return
			}
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
					tunnel := TunnelMap[nc.TunnelId]
					tConn, err := NewTunnelConn(tunnel)
					nConn := GetStream(nc.ConnUid, tunnel.NodeID)
					if err == nil && nConn != nil {
						if tunnel.Type == 2 {
							go UdpBridge(tConn, nConn, tunnel.ID)
						} else {
							go TcpBridge(tConn, nConn, tunnel.ID)
						}
					}
				case string(common.Heartbeat):
					go func() {
						time.Sleep(time.Second * 30)
						if conn == nil {
							time.Sleep(time.Second * 100)
						}
						go common.SendStructType(conn, common.Heartbeat)
					}()
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
func TcpBridge(tConn net.Conn, nConn *smux.Stream, tid int64) {
	if tConn != nil && nConn != nil {
		go func() {
			l := common.CopyBuffer(nConn, tConn)
			if l > 0 {
				common.SendStruct(sConn, common.UpdateFlow, "", common.FlowType{IsUp: true, TunnelId: tid, NodeId: -1, Flow: l})
			}
		}()
		l := common.CopyBuffer(tConn, nConn)
		if l > 0 {
			common.SendStruct(sConn, common.UpdateFlow, "", common.FlowType{IsUp: false, TunnelId: tid, NodeId: -1, Flow: l})
		}
	}
}
func UdpBridge(tConn net.Conn, nConn *smux.Stream, tid int64) {
	defer tConn.Close()
	defer nConn.Close()
	var flow int64
	defer common.SendStruct(sConn, common.UpdateFlow, "", common.FlowType{IsUp: true, TunnelId: tid, NodeId: -1, Flow: flow})
	if tConn != nil && nConn != nil {
		go func() {
			defer tConn.Close()
			defer nConn.Close()
			var flow int64
			defer common.SendStruct(sConn, common.UpdateFlow, "", common.FlowType{IsUp: false, TunnelId: tid, NodeId: -1, Flow: flow})
			buf := make([]byte, 65507)
			for {
				nConn.SetDeadline(time.Now().Add(time.Second * 30))
				n, err := nConn.Read(buf)
				if err == nil {
					l, _ := tConn.Write(buf[0:n])
					flow += int64(l)
				} else {
					break
				}
			}
		}()
		for {
			buf := make([]byte, 65507)
			for {
				n, err := tConn.Read(buf)
				if err == nil {
					l, _ := nConn.Write(buf[0:n])
					flow += int64(l)
				} else {
					break
				}
			}
		}
	}
}
