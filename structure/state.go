package structure

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"os"
	"server/logger"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/pochard/commons/randstr"
)

type (
	State struct {
		NewAccountMap map[uint]map[string]*Account
		AccountMap    map[uint]map[string]*Account
		Mu            sync.RWMutex
	}

	State4Client struct {
		NewRootsVote map[uint]map[string]int //记录各个分片新状态的投票数
		TxNum        int                     //验证的交易数量
	}

	Account struct {
		Id      int
		Shard   uint
		Address string
		Value   int
		Lock    chan struct{} `json:"-"`
	}

	AccountList []Account
)

func (a AccountList) Len() int           { return len(a) }
func (a AccountList) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a AccountList) Less(i, j int) bool { return a[i].Id < a[j].Id }

// CalculateRoot 执行分片计算分片的状态
func (s *State) CalculateRoot() string {
	// logger.AnalysisLogger.Println(s.NewAccountMap)
	jsonString, err := json.Marshal(s.NewAccountMap)
	if err != nil {
		log.Println(err)
		log.Fatalln("计算账户状态Root失败")
	}
	// return sha256.Sum256(jsonString)
	byte32 := sha256.Sum256(jsonString)
	return hex.EncodeToString(byte32[:])
}

//往全局状态中添加账户
// func (s *State) AppendAccount(acc *Account) {

// }

func UpdateAccount(tranblocks TransactionBlock, s *State) {
	//处理内部交易
	// var SuList map[uint][]SuperTransaction
	// SuList := make(map[uint][]SuperTransaction)
	s.NewAccountMap = s.AccountMap
	for shardNum, tran := range tranblocks.SuperList {
		//处理接力交易
		// logger.AnalysisLogger.Printf("位置%v,交易%v", shardNum, tran)
		for _, tx := range tran {
			ExcuteRelay(tx, s, int(shardNum))
		}
	}
	for shardNum, tran := range tranblocks.InternalList {
		for _, tx := range tran {
			ExcuteInteral(tx, s, int(shardNum))
		}
	}
	for shardNum, tran := range tranblocks.CrossShardList {
		//处理跨分片交易
		for _, tx := range tran {
			ExcuteCross(tx, s, int(shardNum))
			// SuList[shardNum] = append(SuList[shardNum], *res)
		}
	}

}

func ExcuteInteral(i InternalTransaction, s *State, shardNum int) {
	if uint(shardNum) != i.Shard {
		log.Printf("节点分片%v, 交易分片%v", shardNum, i.Shard)
		log.Fatalln("该交易不由本分片进行处理")
		return
	}
	Payer := i.From
	Beneficiary := i.To
	Value := i.Value

	_, flag := s.AccountMap[uint(shardNum)][Payer]
	if !flag {
		log.Fatalf("该交易的付款者不是本分片的账户")
		return
	}
	_, flag = s.AccountMap[uint(shardNum)][Beneficiary]
	if !flag {
		log.Fatalf("该交易的收款者不是本分片的账户")
		return
	}

	value1 := s.AccountMap[uint(shardNum)][Payer].Value - Value
	s.NewAccountMap[uint(shardNum)][Payer].Value = value1

	value2 := s.AccountMap[uint(shardNum)][Beneficiary].Value + Value
	s.NewAccountMap[uint(shardNum)][Beneficiary].Value = value2
}

func ExcuteCross(e CrossShardTransaction, s *State, shardNum int) *SuperTransaction {
	if uint(shardNum) != e.Shard1 {
		log.Fatalln("该交易的发起用户不是本分片账户")
		return nil
	}
	Payer := e.From
	_, flag := s.AccountMap[uint(shardNum)][Payer]
	if !flag {
		log.Fatalf("该交易的付款者不是本分片的账户")
		return nil
	}

	s.NewAccountMap[uint(shardNum)][Payer].Value = s.AccountMap[uint(shardNum)][Payer].Value - e.Value
	res := SuperTransaction{
		FromShard: e.Shard1,
		Shard:     e.Shard2,
		From:      e.From,
		To:        e.To,
		Value:     e.Value,
	}
	return &res
}

func ExcuteRelay(r SuperTransaction, s *State, shardNum int) {
	if uint(shardNum) != r.Shard {
		log.Fatalf("该交易不是由本分片执行")
		return
	}
	Beneficiary := r.To
	_, flag := s.AccountMap[uint(shardNum)][Beneficiary]
	if !flag {
		log.Fatalf("该交易的收款者不是本分片的账户")
		return
	}

	s.NewAccountMap[uint(shardNum)][Beneficiary].Value = s.AccountMap[uint(shardNum)][Beneficiary].Value + r.Value
}

// GetAccountList 获取当前所有的账户的状态
func (s *State) GetAccountList() []Account {
	var acc AccountList

	s.Mu.RLock()
	defer s.Mu.RUnlock()
	for i := 1; i <= ShardNum; i++ {
		for _, v := range s.AccountMap[uint(i)] {
			acc = append(acc, *v)
		}
	}
	// Sort the accounts based on your criteria, here we sort by Account.ID
	sort.Sort(acc)
	return acc
}

func (s *State) GetAddressList(shardNum int) []string {
	var addressList []string
	for _, v := range s.AccountMap[uint(shardNum)] {
		addressList = append(addressList, v.Address)
	}
	return addressList
}

// InitAccountList 为执行分片初始化生成n*shardNum个AccountList
func (s *State) InitAccountList(shardNum int, n int) {
	addressList := GenerateAddressList(n)
	for j := 1; j <= shardNum; j++ {
		if s.AccountMap[uint(j)] == nil {
			s.AccountMap[uint(j)] = make(map[string]*Account)
			s.NewAccountMap[uint(j)] = make(map[string]*Account)
		}
		lock := make(chan struct{}, 1)
		lock <- struct{}{}
		for i := 0; i < n; i++ {
			acc := Account{
				Id:      (j-1)*n + i,
				Shard:   uint(j),
				Address: addressList[i+AccountNum*(j-1)],
				Value:   100000, //初始化的Value设置
				Lock:    lock,
			}
			s.AccountMap[uint(j)][acc.Address] = &acc
			s.NewAccountMap[uint(j)][acc.Address] = &acc
			log.Printf("分片%v添加账户成功，账户地址为%v\n", acc.Shard, acc.Address)
		}
	}
}

func GenerateKey() string {
	return randstr.RandomAlphanumeric(16)
}

func GenerateAddressList(n int) []string {
	set := make(map[string]struct{})

	// 创建句柄
	fi, err := os.Open("address.txt")
	if err != nil {
		panic(err)
	}

	// 创建 Reader
	r := bufio.NewReader(fi)

	for len(set) < n*ShardNum {
		line, err := r.ReadString('\n')
		line = strings.TrimSpace(line)
		if err != nil && err != io.EOF {
			panic(err)
		}
		if err == io.EOF {
			break
		}
		// fmt.Println(line)

		key := line
		set[key] = struct{}{}
	}
	var res []string
	for key := range set {
		res = append(res, key)
	}
	return res
}

// InitState 初始化构建所有分片的全局状态
// n表示每个执行分片中需要初始化的账户数目
func InitState(n int, shardNum int) *State {
	state := State{
		NewAccountMap: make(map[uint]map[string]*Account),
		AccountMap:    make(map[uint]map[string]*Account),
		Mu:            sync.RWMutex{},
	}
	return &state
}

// func (s *State) LogState(height uint) {
// 	logger.StateLogger.Printf("当前的区块高度是%v\n", height)
// 	for i := 1; i <= ShardNum; i++ {
// 		for key, acc := range s.AccountMap[uint(i)] {
// 			logger.StateLogger.Printf("账户{%v}的余额为{%v}\n", key, acc.Value)
// 		}
// 	}
// }

func UpdateChainWithBlock(b Block, s *State) {
	// 从数据库中读取交易
	txBlocks := packTx4ServerUpdate()
	// 根据交易（和区块中的交易是对应的）更新账户（NewAccount）
	UpdateAccount(txBlocks, s)
	// 检查收到的root是否超过阈值，超过则用NewAccount替换真实的Account
	VerifyGSRoot(b.Header.StateRoot.Vote, s)
}

func packTx4ServerUpdate() TransactionBlock {
	height := GetHeight()
	IntList := make(map[uint][]InternalTransaction)
	CroList := make(map[uint][]CrossShardTransaction)
	ReList := make(map[uint][]SuperTransaction)
	for shard := 1; shard <= ShardNum; shard++ {
		Int, Cro, Sup, _, _, _ := PackValidateTrans(height-1, shard)
		IntList[uint(shard)] = append(IntList[uint(shard)], Int...)
		CroList[uint(shard)] = append(CroList[uint(shard)], Cro...)
		ReList[uint(shard)] = append(ReList[uint(shard)], Sup...)
	}
	return TransactionBlock{
		Height:         uint(height),
		InternalList:   IntList,
		CrossShardList: CroList,
		SuperList:      ReList,
	}
}

func VerifyGSRoot(vote map[uint]map[string]int, s *State) {
	// MinVote := math.Max(1, math.Floor(2*(CLIENT_MAX-ProposerNum/ShardNum)/3))
	MinVote := CLIENT_MAX * 2 / 3
	isValid := false
	num := 0
	for i := 1; i <= ShardNum; i++ {
		// logger.AnalysisLogger.Printf("分片%v中收到的树根结果为%v", i, vote[uint(i)])
		// logger.AnalysisLogger.Printf("分片%v的Rootsvote为%v", i, s.RootsVote[uint(i)])
		// logger.AnalysisLogger.Printf("分片%v的NewRootsvote为%v", i, s.NewRootsVote[uint(i)])
		for _, votes := range Source.ClientShard[uint(i)].AccountState.NewRootsVote[uint(i)] {
			if votes >= int(MinVote) {
				isValid = true
			}
		}
		if isValid {

			num += Source.ClientShard[uint(i)].AccountState.TxNum
			s.AccountMap[uint(i)] = s.NewAccountMap[uint(i)]
			logger.AnalysisLogger.Printf("树根验证成功,成功上链数量:", num)
		} else {
			logger.AnalysisLogger.Printf("树根验证失败")
		}
		isValid = false
	}
	Source.ChainShard[0].TotalTxNum += num
	tps := float64(Source.ChainShard[0].TotalTxNum) / float64((time.Now().UnixMicro() - Source.ChainShard[0].InitTime)) * 1000000
	blocklatency := (float64((time.Now().UnixMicro() - Source.ChainShard[0].InitTime)) / 1000000) / float64(GetHeight())
	logger.BlockLogger.Printf("第%v个区块的交易完成,验证完成%v条交易,tps:%v,blocklatency:%v", GetHeight()-1, num, tps, blocklatency)
}

func TryLock(account *Account) bool {
	select {
	case <-account.Lock:
		return true
	default:
		return false
	}
}

func Unlock(account *Account) {
	account.Lock <- struct{}{}
}
