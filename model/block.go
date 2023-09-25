package model

import (
	"context"
	"encoding/json"
	"errors"
	"server/logger"
	"server/structure"
	"sync"
	"time"
)

type (
	IdConcurrentMap struct {
		sync.Mutex
		mp      map[string]*sync.Mutex
		keyToCh map[string]chan struct{}
	}

	BlockTransactionRequest struct {
		Shard uint
		//Id    string
	}

	MultiCastProposalRequest struct {
		Shard         uint
		Id            string
		IdList        []string
		ProposalBlock structure.ProposalBlock
	}

	MultiCastProposalResponse struct {
		Message string
	}

	BlockTransactionResponse struct {
		Shard  uint
		Height uint
		Num    int
		// TxRoot         map[uint]string
		InternalList   map[uint][]structure.InternalTransaction
		CrossShardList map[uint][]structure.CrossShardTransaction
		RelayList      map[uint][]structure.SuperTransaction
		Signature      []structure.PubKeySign
	}

	BlockPackedTransactionResponse struct {
		Shard      uint
		Height     uint
		RootString map[uint]string
		Signsmap   map[uint][]structure.PubKeySign
	}

	BlockAccountRequest struct {
		Shard uint
	}

	BlockAccountResponse struct {
		Shard       uint
		Height      uint //当前区块的高度
		AccountList []structure.Account
		GSRoot      structure.GSRoot
	}

	//MessageType = 6
	BlockUploadRequest struct {
		// Shard     uint
		Id     string
		Height uint
		Block  structure.Block
		// ReLayList map[uint][]structure.SuperTransaction
	}

	BlockUploadResponse struct {
		// Shard   uint
		Height  uint
		Message string
	}

	TxWitnessRequest struct {
		Shard uint
	}

	TxWitnessResponse struct {
		// Id             string
		Shard          uint
		Height         uint
		Num            int
		InternalList   map[uint][]structure.InternalTransaction
		CrossShardList map[uint][]structure.CrossShardTransaction
		RelayList      map[uint][]structure.SuperTransaction
		Sign           structure.PubKeySign
	}

	//MessageType = 7
	TxWitnessRequest_2 struct {
		// Id             string
		Shard          uint
		Height         uint
		Num            int
		InternalList   map[uint][]structure.InternalTransaction
		CrossShardList map[uint][]structure.CrossShardTransaction
		RelayList      map[uint][]structure.SuperTransaction
		Sign           structure.PubKeySign
	}

	TxWitnessResponse_2 struct {
		Message string
		Flag    bool
	}

	//MessageType = 8
	RootUploadRequest struct {
		Shard  uint
		Height uint
		TxNum  int
		Id     string
		Root   string
		SuList map[uint][]structure.SuperTransaction
	}

	RootUploadResponse struct {
		// Shard   uint
		Height  uint
		Message string
	}

	GetProposalRequest struct {
		Height   int
		Identity string
	}
	GetProposalResponse struct {
		ProposalBlocks []structure.Block
	}
)

var idLocks = NewIdConcurrentMap()

func NewIdConcurrentMap() *IdConcurrentMap {
	return &IdConcurrentMap{
		mp:      make(map[string]*sync.Mutex),
		keyToCh: make(map[string]chan struct{}),
	}
}

func (m *IdConcurrentMap) Put(k string, v *sync.Mutex) {
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

func (m *IdConcurrentMap) Get(k string, maxWaitingDuration time.Duration) (*sync.Mutex, error) {
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
func PackTransaction(data BlockTransactionRequest) []structure.Proposal {
	var res []structure.Proposal
	shard := data.Shard
	height := structure.GetHeight()

	// 排序节点
	for i := 1; i <= structure.ShardNum; i++ {
		bt1, bt2, bt3 := structure.PackBatchTrans(height+2, i)
		proposal := structure.Proposal{
			Shard:         shard,
			Height:        uint(height) + 1,
			InternalBatch: bt1,
			CrossBatch:    bt2,
			SuperBatch:    bt3,
		}
		res = append(res, proposal)
	}
	return res
}

func AppendBlock(leaderProblock structure.ProposalBlock) (error, bool) {
	var (
		leaderProblockCount int
		OrderClientNum      = structure.ProposerNum
		leaderProblocks     = structure.Source.LeaderProblocksInfo
	)
	structure.Source.LeaderLock.Lock()
	defer structure.Source.LeaderLock.Unlock()

	key := GetProblockKey(leaderProblock) // 根据 proBlock 生成一个唯一标识符

	info, exists := leaderProblocks[key]
	if exists {
		// 如果已经存在，增加计数
		info.Count++
	} else {
		// 否则创建一个新的记录
		info = structure.LeaderProblockInfo{
			Block: leaderProblock,
			Count: 1,
		}
	}

	leaderProblocks[key] = info

	for _, blockInfo := range leaderProblocks {
		leaderProblockCount += blockInfo.Count
	}

	if leaderProblockCount >= OrderClientNum {
		err := CheckLeaderProblocks()
		if err != nil {
			logger.AnalysisLogger.Println("上链失败：", err)
			return err, false
		}
		structure.Source.ChainShard[uint(0)].NodeNum = structure.NewNodeNum()
		for i := 1; i <= structure.ShardNum; i++ {
			structure.Source.ClientShard[uint(i)].Lock.Lock()
			structure.Source.LeaderProblocksInfo = make(map[string]structure.LeaderProblockInfo)
			structure.Source.ClientShard[uint(i)].AccountState.TxNum = 0
			structure.Source.ClientShard[uint(i)].AccountState.NewRootsVote = make(map[uint]map[string]int)
			structure.Source.ClientShard[uint(i)].ValidEnd = false
			for j := 1; j <= structure.ShardNum; j++ {
				structure.Source.ClientShard[uint(i)].AccountState.NewRootsVote[uint(j)] = make(map[string]int)
			}
			structure.Source.ClientShard[uint(i)].Lock.Unlock()
		}
		return nil, true

	} else {
		return nil, false
	}
}

func PackAccount(data BlockAccountRequest) BlockAccountResponse {
	shard := data.Shard
	accountList := structure.Source.ChainShard[0].AccountState.GetAccountList()
	height := structure.GetHeight()

	res := BlockAccountResponse{
		Shard:       shard,
		Height:      uint(height),
		AccountList: accountList,
	}
	return res
}

func MuiltiCastProposal(data structure.ProposalBlock) MultiCastProposalResponse {
	// Marshal the data to payload
	payload, _ := json.Marshal(data)

	metaMessage := MessageMetaData{
		MessageType: 11,
		Message:     payload,
	}

	ComMap := structure.Source.ClientShard[0].ConsensusCommunicationmap

	for _, id := range data.IdList {
		// Acquire or create a lock for this id
		lock, err := idLocks.Get(id, 0)
		if err != nil {
			// Lock doesn't exist, create and store it
			lock = &sync.Mutex{}
			idLocks.Put(id, lock)
		}

		go func(id string, lock *sync.Mutex) {
			client, err := ComMap.Get(id, 0)
			if err != nil {
				return
			}

			lock.Lock()
			err = client.Socket.WriteJSON(metaMessage)
			if err != nil {
				return
			}
			lock.Unlock()

		}(id, lock)
	}

	res := MultiCastProposalResponse{
		Message: "Group multicast proposal succeed",
	}
	return res
}

func PackValidTx(data BlockTransactionRequest) BlockTransactionResponse {
	shard := data.Shard
	height := structure.GetHeight()

	IntList := make(map[uint][]structure.InternalTransaction)
	CroList := make(map[uint][]structure.CrossShardTransaction)
	ReList := make(map[uint][]structure.SuperTransaction)
	if height < 1 {
		res := BlockTransactionResponse{
			Shard:          shard,
			Height:         uint(structure.GetHeight()),
			Num:            0,
			InternalList:   IntList,
			CrossShardList: CroList,
			RelayList:      ReList,
		}
		return res
	}
	Int, Cro, Sup, num1, num2, num3 := structure.PackValidateTrans(height-1, int(shard))
	IntList[shard] = append(IntList[shard], Int...)
	CroList[shard] = append(CroList[shard], Cro...)
	ReList[(shard)] = append(ReList[(shard)], Sup...)

	logger.AnalysisLogger.Printf("执行节点验证阶段打包了%v,%v,%v条交易", num1, num2, num3)
	Num := num1 + num2 + num3
	res := BlockTransactionResponse{
		Shard:          shard,
		Height:         uint(structure.GetHeight()),
		Num:            Num,
		InternalList:   IntList,
		CrossShardList: CroList,
		RelayList:      ReList,
	}
	return res
}

func CollectRoot(data RootUploadRequest) RootUploadResponse {
	height := data.Height
	txNum := data.TxNum
	root := data.Root
	shard := data.Shard
	suList := data.SuList
	structure.Source.ClientShard[shard].Lock.Lock()
	structure.Source.ClientShard[shard].AccountState.NewRootsVote[shard][root] += 1
	logger.AnalysisLogger.Printf("收到来自shard%v的树根投票,该分片的树根以及票数情况为:votes%v", shard, structure.Source.ClientShard[uint(shard)].AccountState.NewRootsVote[shard])

	isEnd := structure.Source.ClientShard[shard].ValidEnd
	flag := false
	for _, num := range structure.Source.ClientShard[shard].AccountState.NewRootsVote[shard] {
		if num >= structure.CLIENT_MAX*2/3 {
			flag = true
		}
	}

	if !isEnd && flag {
		translist := make([]structure.Witnesstrans, 0)

		for index, tran := range suList[shard] {
			trans := structure.Witnesstrans{
				Shard:      int(tran.FromShard),
				ToShard:    int(tran.Shard),
				FromAddr:   tran.From,
				ToAddr:     tran.To,
				TransValue: tran.Value,
				TransType:  2,
			}
			translist = append(translist, trans)
			if index%structure.ValidateTxNum == 0 {
				transType := 2
				structure.AppendBatchTrans(translist, int(shard), transType, structure.Source)
				translist = make([]structure.Witnesstrans, 0)
			}
			structure.AppendWitnessTrans(&trans, 2, int(shard))
		}
		logger.BlockLogger.Printf("分片%v写入%v条relaytx\n", shard, len(suList[shard]))
		structure.Source.ClientShard[shard].AccountState.TxNum += txNum
		logger.BlockLogger.Printf("分片%v验证成功%v条交易\n", shard, txNum)
		structure.Source.ClientShard[shard].ValidEnd = true
	}

	structure.Source.ClientShard[shard].Lock.Unlock()
	res := RootUploadResponse{
		Height:  height,
		Message: "树根上传成功",
	}
	return res
}

func GetProposalBlock(data GetProposalRequest) GetProposalResponse {

	chain := structure.Source.ChainShard[0].Chain
	res := GetProposalResponse{}
	if data.Identity == "order" {
		blockNum := structure.GetHeight()
		if blockNum == 0 {
			res = GetProposalResponse{}

		} else if blockNum >= 3 {
			res = GetProposalResponse{
				ProposalBlocks: chain[data.Height-3:],
			}

		} else {
			res = GetProposalResponse{
				ProposalBlocks: chain,
			}

		}
	} else {
		if data.Height < 1 {
			res = GetProposalResponse{
				ProposalBlocks: []structure.Block{},
			}
			return res
		}
		res = GetProposalResponse{
			ProposalBlocks: chain[data.Height-1 : data.Height],
		}
	}
	return res
}

// GetProblockKey 生成 leaderProblock 的字符串标识
func GetProblockKey(leaderProblock structure.ProposalBlock) string {
	stringJSON, err := json.Marshal(leaderProblock)
	if err != nil {
		return ""
	}
	return string(stringJSON)
}

// CheckLeaderProblocks 检查是否已经达到 OrderClientNum 个 leaderProblock
func CheckLeaderProblocks() error {
	var (
		maxCount        int
		maxProblockInfo structure.LeaderProblockInfo
		leaderProblocks = structure.Source.LeaderProblocksInfo
		OrderClientNum  = structure.ProposerNum
		Chain           = structure.Source.ChainShard[0]
	)

	for _, info := range leaderProblocks {
		if info.Count > maxCount {
			maxCount = info.Count
			maxProblockInfo = info
		}
	}

	if maxCount >= (2*OrderClientNum)/3 {
		Block := structure.MakeBlock(maxProblockInfo.Block, uint(maxProblockInfo.Block.Height), maxProblockInfo.Block.Root)
		Block.Header.Vote = uint(maxCount)
		err := Chain.VerifyBlock(Block)
		if err != nil {
			logger.AnalysisLogger.Println("VerifyBlock Error: ", err)
			return err
		}
		structure.Source.ChainShard[0].Chain = append(structure.Source.ChainShard[uint(0)].Chain, Block)

		err = Chain.AppendBlock(Block)
		if err != nil {
			return err
		}
	} else {
		return errors.New("收到投票数不足")
	}
	return nil
}
