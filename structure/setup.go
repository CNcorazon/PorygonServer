package structure

import (
	"os"
	"strconv"
)

const (
	MAX        = 1000000000
	ShardNum1  = 2
	AccountNum = 2
	CLIENT_MAX = 10 // 每个分片中的OrderClients数量

	TX_NUM1      = 4000 //per shard per catagory
	ProposerNum1 = 10   // 总共的OrderClients数量

	SIGN_VERIFY_TIME = 4 //millisecond
	ICMPCOUNT        = 3
	INGTIME          = 300
	ServerNum        = 1
	ServerIP         = "192.168.199.102" //,172.19.3.234" //,192.168.199.121"
	ServerPort       = ":8088"           //,:8088,:8888"
	WsRequest        = "/forward/wsRequest"

	GORUNTINE_MAX = 2000
)

var Source *Controller

var ShardNum int
var ProposerNum int
var TX_NUM int

func init() {
	ShardNum = ShardNum1
	ProposerNum = ProposerNum1
	TX_NUM = TX_NUM1

	// 如果命令行参数存在，尝试将其转换为整数并修改 ModifiedValue
	if len(os.Args) > 1 {
		ModifiedShardNum, err := strconv.Atoi(os.Args[1])
		if err == nil {
			ShardNum = ModifiedShardNum
			// log.Println(ShardNum)
		}
		ModifiedProposerNum, err := strconv.Atoi(os.Args[2])
		if err == nil {
			ProposerNum = ModifiedProposerNum
			// log.Println(ProposerNum)
		}
		ModifiedTXNUM, err := strconv.Atoi(os.Args[3])
		if err == nil {
			TX_NUM = ModifiedTXNUM
			// log.Println(TX_NUM)
		}
	}
	Source = InitController(ShardNum, AccountNum)
}
