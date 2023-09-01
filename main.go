package main

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/go-ping/ping"
	"io"
	"log"
	"net"
	"os"
	"server/logger"
	request "server/request"
	"server/route"
	"server/structure"
	"strings"
	"sync"
	"time"
)

type (
	ServerIPList struct {
		ServerIP   map[string]bool   //是否能ping通
		ServerPort map[string]string //服务器端口
		ServerCon  map[string]bool   //是否建立连接
		Lock       sync.RWMutex
	}
)

func ServerPing(target string) bool {
	pinger, err := ping.NewPinger(target)
	fmt.Println("test")
	if err != nil {
		panic(err)
	}
	pinger.Count = structure.ICMPCOUNT
	pinger.Timeout = time.Duration(structure.INGTIME * time.Millisecond)
	// pinger.SetPrivileged(true)
	pinger.Run() // blocks until finished
	stats := pinger.Statistics()
	fmt.Println(stats)
	// 有回包，就是说明IP是可用的
	if stats.PacketsRecv >= 1 {
		return true
	} else {
		return false
	}
}

func GetLocalIP() []string {
	var ipStr []string
	netInterfaces, err := net.Interfaces()
	if err != nil {
		fmt.Println("net.Interfaces error:", err.Error())
		return ipStr
	}

	for i := 0; i < len(netInterfaces); i++ {
		if (netInterfaces[i].Flags & net.FlagUp) != 0 {
			addrs, _ := netInterfaces[i].Addrs()
			for _, address := range addrs {
				if ipnet, ok := address.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
					//获取IPv6
					/*if ipnet.IP.To16() != nil {
						fmt.Println(ipnet.IP.String())
						ipStr = append(ipStr, ipnet.IP.String())

					}*/
					//获取IPv4
					if ipnet.IP.To4() != nil {
						fmt.Println(ipnet.IP.String())
						ipStr = append(ipStr, ipnet.IP.String())

					}
				}
			}
		}
	}
	return ipStr
}

func InitServer() *ServerIPList {
	//与服务器建立连接
	structure.Source.Read_Server_CommnicationMap = make(map[string]*structure.Server, 3)
	// structure.Source.Read_Server_Validate_CommnicationMap = make(map[string]*structure.Server, 3)
	// var comm2 *websocket.Conn
	// var comm3 *websocket.Conn
	ServerIPList := ServerIPList{
		ServerIP:   make(map[string]bool, structure.ServerNum),
		ServerCon:  make(map[string]bool, structure.ServerNum),
		ServerPort: make(map[string]string, structure.ServerNum),
		Lock:       sync.RWMutex{},
	}
	IP := strings.Split(structure.ServerIP, ",")
	Port := strings.Split(structure.ServerPort, ",")
	SelfIP := GetLocalIP()
	fmt.Println(SelfIP)
	ServerIPList.Lock.Lock()
	for i := 0; i < structure.ServerNum; i++ {
		ServerIPList.ServerPort[IP[i]] = Port[i]
		if IP[i] != SelfIP[0] {
			if ServerPing(IP[i]) {
				ServerIPList.ServerIP[IP[i]] = true
			} else {
				ServerIPList.ServerIP[IP[i]] = false
			}
			ServerIPList.ServerCon[IP[i]] = false
		}
	}
	log.Println(ServerIPList.ServerIP)
	ServerIPList.Lock.Unlock()
	log.Printf("------与已开启的服务器建立通讯------")
	// time.Sleep(30 * time.Second) //等待30秒让我开服务器。。。
	for ip, _ := range ServerIPList.ServerIP {
		WSURL := "ws://" + ip + ServerIPList.ServerPort[ip]
		conn, flag := request.WSRequest(WSURL, WsRequest, GetLocalIP()[0])
		if !flag {
			logger.AnalysisLogger.Printf("服务器%v尚未开启", ip)
		} else {
			server := &structure.Server{
				Ip:     ip,
				Socket: conn,
				Lock:   sync.RWMutex{},
			}
			structure.Source.Read_Server_CommnicationMap[ip] = server
			structure.Source.Read_Server_CommnicationMap[ip].Socket.SetReadDeadline(time.Time{})
			ServerIPList.ServerCon[ip] = true
			ServerIPList.ServerIP[ip] = true
			logger.AnalysisLogger.Printf("已和服务器%v建立通讯", ip)
		}
	}
	return &ServerIPList
}

const (
	// WSURL1    = "ws://192.168.1.8:8080"     //jianting
	// WSURL2    = "ws://192.168.199.157:8080" //junyuan
	// WSURL3    = "ws://192.168.199.121:8080" //yusheng
	WsRequest = "/forward/wsRequest"
	// ClientForward = "/forward/clientRegister "
)

func main() {
	gin.DisableConsoleColor()
	f, _ := os.Create("gin.log")
	gin.DefaultWriter = io.MultiWriter(f)

	log.SetFlags(log.Lshortfile | log.LstdFlags)
	r := route.InitRoute()
	r.Run(structure.ServerPort)
}
