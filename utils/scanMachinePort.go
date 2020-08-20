package utils

import (
	"net"
	"strconv"
)

func ScanPort(Ip string,Port int) bool {
	listen, err := net.Listen("tcp", Ip+":"+strconv.Itoa(Port))
	if err==nil{
		listen.Close()
		return false
	}
	return true
}

/**
扫描一个端口是否被占用
*/
func ScanPortOne(Port int) bool {
	return ScanPort("0.0.0.0",Port)
}