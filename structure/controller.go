package structure

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

type (
	Controller struct {
		Shard                         uint                        //Controller控制的总Shard数目
		ChainShard                    map[uint]*HorizonBlockChain //链信息
		ClientShard                   map[uint]*ShardStruct
		LeaderProblocksInfo           map[string]LeaderProblockInfo
		LeaderLock                    sync.Mutex
		Read_Server_CommnicationMap   map[string]*Server
		Write_Server_CommunicationMap map[string]*Server
	}

	LeaderProblockInfo struct {
		Block ProposalBlock
		Count int
	}
	MapClient struct {
		Id     string //服务器生成的证明该连接的身份
		Shard  uint   //该客户端被划归的分片名称
		Socket *websocket.Conn
	}

	Server struct {
		Ip     string
		Socket *websocket.Conn
		Lock   sync.RWMutex
	}

	ShardStruct struct {
		Lock                            sync.Mutex
		Consensus_CommunicationMap      *MyConcurrentMap
		Validation_CommunicationMap     *MyConcurrentMap
		New_Validation_CommunicationMap *MyConcurrentMap
		AddressLsistMap                 []string
		AccountState                    *State4Client
		ValidEnd                        bool
	}

	MyConcurrentMap struct {
		sync.Mutex
		mp      map[string]*MapClient
		keyToCh map[string]chan struct{}
	}
)

var ChainDB *gorm.DB  // 区块信息
var TransDB *gorm.DB  // 交易信息
var ClientDB *gorm.DB // 用户相关信息
// var RedisWitness *redis.Client
var err error

func InitController(shardNum int, accountNum int) *Controller {
	log.Println("/////////初始化数据库/////////")
	dsn1 := "root:123456@tcp(127.0.0.1:3306)/chain?charset=utf8mb4&parseTime=True&loc=Local"
	ChainDB, err = gorm.Open(mysql.Open(dsn1), &gorm.Config{})
	if err != nil {
		fmt.Println(err)
	}
	dsn2 := "root:123456@tcp(127.0.0.1:3306)/horizon?charset=utf8mb4&parseTime=True&loc=Local"
	TransDB, err = gorm.Open(mysql.Open(dsn2), &gorm.Config{})
	if err != nil {
		fmt.Println(err)
	}
	dsn3 := "root:123456@tcp(127.0.0.1:3306)/client?charset=utf8mb4&parseTime=True&loc=Local"
	ClientDB, err = gorm.Open(mysql.Open(dsn3), &gorm.Config{})
	if err != nil {
		fmt.Println(err)
	}

	log.Println("/////////初始化区块链结构/////////")
	controller := Controller{
		Shard:               uint(shardNum),
		ChainShard:          make(map[uint]*HorizonBlockChain, 1),
		ClientShard:         make(map[uint]*ShardStruct),
		LeaderProblocksInfo: make(map[string]LeaderProblockInfo),
		LeaderLock:          sync.Mutex{},
	}
	log.Printf("/////////本系统生成了%v个分片/////////", shardNum)
	chain := MakeHorizonBlockChain(accountNum, shardNum)
	controller.ChainShard[uint(0)] = chain

	controller.ClientShard[0] = NewShardStruct()
	for i := 1; i <= shardNum; i++ {
		shard := NewShardStruct()
		controller.ClientShard[uint(i)] = shard
		controller.ClientShard[uint(i)].AddressLsistMap = chain.AccountState.GetAddressList(i)
	}

	var count int64
	TransDB.Model(&Witnesstrans{}).Count(&count)
	TransDB.AutoMigrate(&Witnesstrans{}, &Batchtrans{})
	if count == 0 {
		TransDB.Exec("DELETE FROM witnesstrans")
		TransDB.Exec("ALTER TABLE witnesstrans AUTO_INCREMENT = 1")
		TransDB.Exec("DELETE FROM batchtrans")
		TransDB.Exec("ALTER TABLE batchtrans AUTO_INCREMENT = 1")
		log.Println("/////////交易池为空，开始初始化交易/////////")
		tranNum := TX_NUM * ShardNum
		// croRate := 0.5 //跨分片交易占据总交易的1/croRate
		start := time.Now()
		if shardNum == 1 {
			//如果只有一个分片，则只需要制作内部交易
			addressList := controller.ClientShard[uint(1)].AddressLsistMap
			translist := make([]Witnesstrans, 0)
			count1 := 0
			for j := 0; j < tranNum; j++ {
				Value := 1
				n1, n2 := GetTwoRand(len(addressList))
				trans := MakeInternalTransaction(1, addressList[n1], addressList[n2], Value)
				count1++
				translist = append(translist, trans)
				if count1 == 2000 {
					AppendTransactionBatch(translist, 1, 0)
					count1 = 0
					translist = make([]Witnesstrans, 0)
				}
				AppendTransaction(trans)
			}
		} else {
			//如果有多个分片
			//先制作一些内部交易
			// intTranNum := tranNum - int((float64(tranNum) * croRate))
			count1 := 0
			translist := make([]Witnesstrans, 0)
			for i := 1; i <= shardNum; i++ {
				addressList := controller.ClientShard[uint(i)].AddressLsistMap
				for j := 0; j < (tranNum / shardNum); j++ {
					Value := 1
					n1, n2 := GetTwoRand(len(addressList))
					trans := MakeInternalTransaction(i, addressList[n1], addressList[n2], Value)
					count1++
					translist = append(translist, trans)
					if count1 == 2000 {
						AppendTransactionBatch(translist, i, 0)
						count1 = 0
						translist = make([]Witnesstrans, 0)
					}
					AppendTransaction(trans)
				}
			}

			//再制作一些跨分片交易
			// croTranNum := int((float64(tranNum) * croRate))
			count1 = 0
			translist = make([]Witnesstrans, 0)
			for i := 1; i <= shardNum; i++ {
				from := i
				target := i + 1
				if i == shardNum {
					target = 1
				}
				addressList1 := controller.ClientShard[uint(from)].AddressLsistMap
				addressList2 := controller.ClientShard[uint(target)].AddressLsistMap
				for i := 0; i < (tranNum / shardNum); i++ {
					Value := 1
					rand.Seed(time.Now().UnixNano())
					n1 := rand.Intn(len(addressList1))
					n2 := rand.Intn(len(addressList2))
					trans := MakeCrossShardTransaction(from, target, addressList1[n1], addressList2[n2], Value)
					count1++
					translist = append(translist, trans)
					if count1 == 2000 {
						AppendTransactionBatch(translist, i, 1)
						count1 = 0
						translist = make([]Witnesstrans, 0)
					}
					AppendTransaction(trans)

				}
			}
		}
		t := time.Since(start)
		fmt.Println(t)
	} else {
		log.Printf("/////////交易池已满，无需初始化/////////")
	}

	log.Println("/////////初始化区块链/////////")
	ChainDB.AutoMigrate(&Blocks{})
	ChainDB.Exec("DELETE FROM blocks")
	err = ChainDB.Exec("SET SESSION TRANSACTION ISOLATION LEVEL SERIALIZABLE").Error
	if err != nil {
		panic("failed to set transaction isolation level")
	}
	log.Println("/////////初始化委员会客户端信息/////////")
	ClientDB.AutoMigrate(&Clients{})
	ClientDB.Exec("DELETE FROM clients")
	ClientDB.Exec("ALTER TABLE clients AUTO_INCREMENT = 1")
	err = ClientDB.Exec("SET SESSION TRANSACTION ISOLATION LEVEL SERIALIZABLE;").Error
	if err != nil {
		panic("failed to set transaction isolation level")
	}
	log.Println("/////////初始化完成/////////")
	return &controller
}

func NewShardStruct() *ShardStruct {
	state4client := State4Client{
		NewRootsVote: make(map[uint]map[string]int),
		Tx_num:       0,
	}
	for i := 1; i <= ShardNum; i++ {
		state4client.NewRootsVote[uint(i)] = make(map[string]int)
	}
	return &ShardStruct{
		Lock:                            sync.Mutex{},
		Consensus_CommunicationMap:      NewMyConcurrentMap(),
		Validation_CommunicationMap:     NewMyConcurrentMap(),
		New_Validation_CommunicationMap: NewMyConcurrentMap(),
		AddressLsistMap:                 make([]string, 0),
		AccountState:                    &state4client,
		ValidEnd:                        false,
	}
}

func NewMyConcurrentMap() *MyConcurrentMap {
	return &MyConcurrentMap{
		mp:      make(map[string]*MapClient),
		keyToCh: make(map[string]chan struct{}),
	}
}

func (m *MyConcurrentMap) Put(k string, v *MapClient) {
	m.Lock()
	defer m.Unlock()
	m.mp[k] = v

	ch, ok := m.keyToCh[k]
	if !ok {
		return
	}

	select {
	case <-ch:
		return
	default:
		close(ch)
	}
}
func (m *MyConcurrentMap) Get(k string, maxWaitingDuration time.Duration) (*MapClient, error) {
	m.Lock()
	v, ok := m.mp[k]
	if ok {
		m.Unlock()
		return v, nil
	}

	ch, ok := m.keyToCh[k]
	if !ok {
		ch = make(chan struct{})
		m.keyToCh[k] = ch
	}

	tCtx, cancel := context.WithTimeout(context.Background(), maxWaitingDuration)
	defer cancel()

	m.Unlock()
	select {
	case <-tCtx.Done():
		return nil, tCtx.Err()
	case <-ch:
	}
	m.Lock()
	v = m.mp[k]
	m.Unlock()
	return v, nil
}

func GetTwoRand(n int) (int, int) {
	rand.Seed(time.Now().UnixNano())
	random1 := rand.Intn(n)
	random2 := rand.Intn(n)

	// 重新生成随机数直到两个数不同
	for random1 == random2 {
		random2 = rand.Intn(n)
	}
	return random1, random2
}

//func createTable() {
//	query := `
//        CREATE TABLE IF NOT EXISTS blacklist (
//            id VARCHAR(255) PRIMARY KEY,
//            reason VARCHAR(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci,
//            personName VARCHAR(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci,
//        	IsIntersect  INT
//            );
//    `
//	_, err := .Exec(query)
//	if err != nil {
//		log.Fatal(err)
//	}
//}
