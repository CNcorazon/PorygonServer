package structure

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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
		Id     string // 以pubKey为Id
		Shard  uint   // 该移动节点所处的分片
		Socket *websocket.Conn
	}

	Server struct {
		Ip     string
		Socket *websocket.Conn
		Lock   sync.RWMutex
	}

	ShardStruct struct {
		Lock                       sync.Mutex
		ConsensusCommunicationmap  *MyConcurrentMap
		ValidationCommunicationmap *MyConcurrentMap
		AddressListMap             []string
		AccountState               *State4Client
		ValidEnd                   bool
	}

	MyConcurrentMap struct {
		sync.Mutex
		mp      map[string]*MapClient
		keyToCh map[string]chan struct{}
	}
)

var (
	ChainDB *gorm.DB // 区块信息
	//TransDB             *gorm.DB // 交易信息
	ClientDB            *gorm.DB //用户相关信息
	err                 error
	WitnessTransactions [][][]Witnesstrans
	BatchTransactions   [][][]Batchtrans
)

func InitController(shardNum int, accountNum int) *Controller {
	log.Println("/////////初始化数据库/////////")
	dsn1 := "root:123456@tcp(127.0.0.1:3306)/chain?charset=utf8mb4&parseTime=True&loc=Local"
	ChainDB, err = gorm.Open(mysql.Open(dsn1), &gorm.Config{})
	if err != nil {
		fmt.Println(err)
	}
	//dsn2 := "root:123456@tcp(127.0.0.1:3306)/horizon?charset=utf8mb4&parseTime=True&loc=Local"
	//TransDB, err = gorm.Open(mysql.Open(dsn2), &gorm.Config{})
	//if err != nil {
	//	fmt.Println(err)
	//}
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
		controller.ClientShard[uint(i)].AddressListMap = chain.AccountState.GetAddressList(i)
	}

	//var count int64
	//TransDB.Exec("DELETE FROM witnesstrans")
	//TransDB.Exec("DELETE FROM batchtrans")
	//TransDB.Model(&Witnesstrans{}).Count(&count)
	//TransDB.AutoMigrate(&Witnesstrans{}, &Batchtrans{})
	//if count == 0 {
	//	TransDB.Exec("ALTER TABLE witnesstrans AUTO_INCREMENT = 1")
	//	TransDB.Exec("ALTER TABLE batchtrans AUTO_INCREMENT = 1")
	//	log.Println("/////////交易池为空，开始初始化交易/////////")
	//	tranNum := TX_NUM * ShardNum
	//	// croRate := 0.5 //跨分片交易占据总交易的1/croRate
	//	start := time.Now()
	//	if shardNum == 1 {
	//		//如果只有一个分片，则只需要制作内部交易
	//		addressList := controller.ClientShard[uint(1)].AddressListMap
	//		translist := make([]Witnesstrans, 0)
	//		count1 := 0
	//		for j := 0; j < tranNum; j++ {
	//			Value := 1
	//			n1, n2 := GetTwoRand(len(addressList))
	//			trans := MakeInternalTransaction(1, addressList[n1], addressList[n2], Value)
	//			count1++
	//			translist = append(translist, trans)
	//			if count1 == 2000 {
	//				AppendTransactionBatch(translist, 1, 0)
	//				count1 = 0
	//				translist = make([]Witnesstrans, 0)
	//			}
	//			AppendTransaction(trans)
	//		}
	//	} else {
	//		//如果有多个分片
	//		//先制作一些内部交易
	//		// intTranNum := tranNum - int((float64(tranNum) * croRate))
	//		count1 := 0
	//		translist := make([]Witnesstrans, 0)
	//		for i := 1; i <= shardNum; i++ {
	//			addressList := controller.ClientShard[uint(i)].AddressListMap
	//			for j := 0; j < (tranNum / shardNum); j++ {
	//				Value := 1
	//				n1, n2 := GetTwoRand(len(addressList))
	//				trans := MakeInternalTransaction(i, addressList[n1], addressList[n2], Value)
	//				count1++
	//				translist = append(translist, trans)
	//				if count1 == 2000 {
	//					AppendTransactionBatch(translist, i, 0)
	//					count1 = 0
	//					translist = make([]Witnesstrans, 0)
	//				}
	//				AppendTransaction(trans)
	//			}
	//		}
	//
	//		//再制作一些跨分片交易
	//		// croTranNum := int((float64(tranNum) * croRate))
	//		count1 = 0
	//		translist = make([]Witnesstrans, 0)
	//		for i := 1; i <= shardNum; i++ {
	//			from := i
	//			target := i + 1
	//			if i == shardNum {
	//				target = 1
	//			}
	//			addressList1 := controller.ClientShard[uint(from)].AddressListMap
	//			addressList2 := controller.ClientShard[uint(target)].AddressListMap
	//			for i := 0; i < (tranNum / shardNum); i++ {
	//				Value := 1
	//				rand.Seed(time.Now().UnixNano())
	//				n1 := rand.Intn(len(addressList1))
	//				n2 := rand.Intn(len(addressList2))
	//				trans := MakeCrossShardTransaction(from, target, addressList1[n1], addressList2[n2], Value)
	//				count1++
	//				translist = append(translist, trans)
	//				if count1 == 2000 {
	//					AppendTransactionBatch(translist, i, 1)
	//					count1 = 0
	//					translist = make([]Witnesstrans, 0)
	//				}
	//				AppendTransaction(trans)
	//			}
	//		}
	//	}
	//	t := time.Since(start)
	//	fmt.Println(t)
	//} else {
	//	log.Printf("/////////交易池已满，无需初始化/////////")
	//}
	log.Println("/////////开始初始化交易/////////")
	initializeWitnessTransactions(2, 2)
	initializeBatchTransactions(2, 2)
	initializeTransactions(TX_NUM*ShardNum, ShardNum, &controller)

	log.Println("/////////初始化区块链/////////")
	ChainDB.AutoMigrate(&Blocks{})
	ChainDB.Exec("DELETE FROM blocks")
	err = ChainDB.Exec("SET SESSION TRANSACTION ISOLATION LEVEL SERIALIZABLE").Error
	if err != nil {
		panic("failed to set transaction isolation level")
	}
	log.Println("/////////初始化委员会信息/////////")
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
		TxNum:        0,
	}
	for i := 1; i <= ShardNum; i++ {
		state4client.NewRootsVote[uint(i)] = make(map[string]int)
	}
	return &ShardStruct{
		Lock:                       sync.Mutex{},
		ConsensusCommunicationmap:  NewMyConcurrentMap(),
		ValidationCommunicationmap: NewMyConcurrentMap(),
		//New_Validation_CommunicationMap: NewMyConcurrentMap(),
		AddressListMap: make([]string, 0),
		AccountState:   &state4client,
		ValidEnd:       false,
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

func initializeTransactions(tranNum, shardNum int, controller *Controller) {
	log.Println("/////////交易池为空，开始初始化交易/////////")
	// croRate := 0.5 //跨分片交易占据总交易的1/croRate
	if shardNum == 1 {
		//如果只有一个分片，则只需要制作内部交易
		addressList := controller.ClientShard[uint(1)].AddressListMap
		translist := make([]Witnesstrans, 0)
		count1 := 0
		for j := 0; j < tranNum; j++ {
			trans := generateInterTrans(addressList, shardNum)
			count1++
			translist = append(translist, trans)
			if count1 == ValidateTxNum {
				transType := 0
				AppendBatchTrans(translist, shardNum, transType, controller)
				count1 = 0
				translist = make([]Witnesstrans, 0)
			}
			AppendWitnessTrans(&trans, 0, shardNum)
		}
	} else {
		//如果有多个分片
		//先制作一些内部交易
		// intTranNum := tranNum - int((float64(tranNum) * croRate))
		count1 := 0
		translist := make([]Witnesstrans, 0)
		for i := 1; i <= shardNum; i++ {
			addressList := controller.ClientShard[uint(i)].AddressListMap
			for j := 0; j < (tranNum / shardNum); j++ {
				trans := generateInterTrans(addressList, i)
				count1++
				translist = append(translist, trans)
				if count1 == ValidateTxNum {
					transType := 0
					AppendBatchTrans(translist, i, transType, controller)
					count1 = 0
					translist = make([]Witnesstrans, 0)
				}
				AppendWitnessTrans(&trans, 0, i)
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
			addressList1 := controller.ClientShard[uint(from)].AddressListMap
			addressList2 := controller.ClientShard[uint(target)].AddressListMap
			for j := 0; j < (tranNum / shardNum); j++ {
				trans := generateCrossTrans(addressList1, addressList2, from, target)
				count1++
				translist = append(translist, trans)
				if count1 == ValidateTxNum {
					transType := 1
					AppendBatchTrans(translist, i, transType, controller)
					count1 = 0
					translist = make([]Witnesstrans, 0)
				}
				AppendWitnessTrans(&trans, 1, i)
			}
		}
	}
}

func AppendWitnessTrans(witnesstrans *Witnesstrans, transType int, shard int) {
	WitnessTransactions[transType][shard] = append(WitnessTransactions[transType][shard], *witnesstrans)
}

func AppendBatchTrans(transList []Witnesstrans, shard, transType int, controller *Controller) {
	jsonByte, _ := json.Marshal(transList)
	byte32 := sha256.Sum256(jsonByte)
	Abstring := hex.EncodeToString(byte32[:])
	accounts := controller.ChainShard[0].AccountState.GetAccountList()
	relatedAccounts := FindAccountIds(accounts, transList)
	batchtrans := &Batchtrans{
		Shard:          shard,
		Abstract:       Abstring,
		RelatedAccount: relatedAccounts,
		TransType:      transType,
	}
	BatchTransactions[transType][shard] = append(BatchTransactions[transType][shard], *batchtrans)
}

func FindAccountIds(accounts []Account, witnessTrans []Witnesstrans) []int {
	// 创建映射，将 Address 映射到对应的 Account Id
	accountAddressMap := make(map[string]int)
	for _, acc := range accounts {
		accountAddressMap[acc.Address] = acc.Id
	}
	// 使用 map 来确保结果中的 Id 是唯一的
	uniqueIds := make(map[int]struct{})

	// 遍历 WitnessTrans，查找 FromAddr 和 ToAddr 对应的 Account Id
	for _, wt := range witnessTrans {
		if wt.TransType == 0 {
			fromId, ok := accountAddressMap[wt.FromAddr]
			if ok {
				uniqueIds[fromId] = struct{}{}
			}
			toId, ok := accountAddressMap[wt.ToAddr]
			if ok {
				uniqueIds[toId] = struct{}{}
			}
		} else if wt.TransType == 1 {
			fromId, ok := accountAddressMap[wt.FromAddr]
			if ok {
				uniqueIds[fromId] = struct{}{}
			}
		} else {
			toId, ok := accountAddressMap[wt.ToAddr]
			if ok {
				uniqueIds[toId] = struct{}{}
			}
		}
	}

	// 将唯一的 Id 放入结果切片中
	var accountIds []int

	for id := range uniqueIds {
		accountIds = append(accountIds, id)
	}

	//logger.AnalysisLogger.Println("涉及的账户包括：", accountIds)
	return accountIds
}

func PackBatchTrans(height, shard int) ([]Batchtrans, []Batchtrans, []Batchtrans) {
	start := height % len(BatchTransactions[0][shard])
	end := (height + 1) % len(BatchTransactions[0][shard])
	if end == 0 {
		end = len(BatchTransactions[0][shard])
	}
	sliceLength := len(BatchTransactions[2][shard])
	Bt := []Batchtrans{}
	if sliceLength >= 1 {
		startIdx := sliceLength - 1
		endIdx := sliceLength
		Bt = BatchTransactions[2][shard][startIdx:endIdx]
	}
	return BatchTransactions[0][shard][start:end], BatchTransactions[1][shard][start:end], Bt
}

func PackValidateTrans(height, shard int) ([]InternalTransaction, []CrossShardTransaction, []SuperTransaction, int, int, int) {
	var InterTransList []InternalTransaction
	var CrossTransList []CrossShardTransaction
	var SuperTransList []SuperTransaction
	start := (height * ValidateTxNum) % len(WitnessTransactions[0][shard])
	end := ((height + 1) * ValidateTxNum) % len(WitnessTransactions[0][shard])
	if end == 0 {
		end = len(WitnessTransactions[0][shard])
	}
	sliceLength := len(WitnessTransactions[2][shard])
	SuperWT := []Witnesstrans{}
	if sliceLength >= ValidateTxNum {
		startIdx := sliceLength - ValidateTxNum
		endIdx := sliceLength
		SuperWT = WitnessTransactions[2][shard][startIdx:endIdx]
	}
	InterWT := WitnessTransactions[0][shard][start:end]
	CrossWT := WitnessTransactions[1][shard][start:end]
	for _, rawtrans := range InterWT {
		tran := InternalTransaction{
			Shard: uint(rawtrans.Shard),
			From:  rawtrans.FromAddr,
			To:    rawtrans.ToAddr,
			Value: rawtrans.TransValue,
		}
		InterTransList = append(InterTransList, tran)
	}
	for _, rawtrans := range CrossWT {
		tran := CrossShardTransaction{
			Shard1: uint(rawtrans.Shard),
			Shard2: uint(rawtrans.ToShard),
			From:   rawtrans.FromAddr,
			To:     rawtrans.ToAddr,
			Value:  rawtrans.TransValue,
		}
		CrossTransList = append(CrossTransList, tran)
	}
	for _, rawtrans := range SuperWT {
		tran := SuperTransaction{
			FromShard: uint(rawtrans.Shard),
			Shard:     uint(rawtrans.ToShard),
			From:      rawtrans.FromAddr,
			To:        rawtrans.ToAddr,
			Value:     rawtrans.TransValue,
		}
		SuperTransList = append(SuperTransList, tran)
	}
	num1 := len(InterWT)
	num2 := len(CrossWT)
	num3 := len(SuperWT)
	return InterTransList, CrossTransList, SuperTransList, num1, num2, num3
}

func generateInterTrans(addressList []string, shardNum int) Witnesstrans {
	Value := 1
	n1, n2 := GetTwoRand(len(addressList))
	trans := MakeInternalTransaction(shardNum, addressList[n1], addressList[n2], Value)
	return trans
}

func generateCrossTrans(addressList1, addressList2 []string, from, target int) Witnesstrans {
	Value := 1
	rand.Seed(time.Now().UnixNano())
	n1 := rand.Intn(len(addressList1))
	n2 := rand.Intn(len(addressList2))
	trans := MakeCrossShardTransaction(from, target, addressList1[n1], addressList2[n2], Value)
	return trans
}

func initializeWitnessTransactions(transType, shardNum int) {
	// Ensure the first dimension of the slice has enough capacity
	for len(WitnessTransactions) <= transType {
		WitnessTransactions = append(WitnessTransactions, make([][]Witnesstrans, 0))
	}
	// Ensure the second dimension of the slice has enough capacity
	for i := 0; i <= transType; i++ {
		for len(WitnessTransactions[i]) <= shardNum {
			WitnessTransactions[i] = append(WitnessTransactions[i], make([]Witnesstrans, 0))
		}
	}

}

func initializeBatchTransactions(transType, shardNum int) {
	// Ensure the first dimension of the slice has enough capacity
	for len(BatchTransactions) <= transType {
		BatchTransactions = append(BatchTransactions, make([][]Batchtrans, 0))
	}
	// Ensure the second dimension of the slice has enough capacity
	for i := 0; i <= transType; i++ {
		for len(BatchTransactions[i]) <= shardNum {
			BatchTransactions[i] = append(BatchTransactions[i], make([]Batchtrans, 0))
		}
	}
}
