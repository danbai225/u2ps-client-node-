package main

import (
	"flag"
	"log"
	"u2ps/client"
	"u2ps/common"
	"u2ps/server"
)

func main() {
	log.Println("版本:",common.Versions)
	flag.IntVar(&common.MaxRi, "r", 10, "最多重连次数")
	flag.StringVar(&common.HostInfo, "h", "server.u2ps.com:2251", "连接地址")
	flag.StringVar(&common.Token, "t", "", "连接认证Token")
	flag.BoolVar(&common.NodeMode, "n", false, "Node模式")
	flag.StringVar(&common.Key, "k", "", "客户端Key")
	flag.Parse()
	if common.NodeMode {
		log.Println("节点模式运行...")
		server.Conn()
	} else {
		log.Println("客户端模式运行...")
		client.Conn()
	}

}