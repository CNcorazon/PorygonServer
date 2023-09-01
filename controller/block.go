package controller

import (
	"encoding/json"
	"errors"
	"github.com/gin-gonic/gin"
	"log"
	"net/http"
	"server/logger"
	"server/model"
	"server/structure"
	"sync"
)

var (
	mu sync.Mutex
)

func PackTransaction(c *gin.Context) {
	var (
		data model.BlockTransactionRequest
		res  []structure.Proposal
	)

	if err := c.ShouldBindJSON(&data); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	shard := data.Shard
	height := structure.GetHeight()

	if shard == 0 {
		// 排序节点
		for i := 1; i <= structure.ShardNum; i++ {
			bt1, bt2, bt3 := structure.PackTransactionBatch(height, int(i))
			proposal := structure.Proposal{
				Shard:         shard,
				Height:        uint(height) + 1,
				InternalBatch: bt1,
				CrossBatch:    bt2,
				SuperBatch:    bt3,
			}
			res = append(res, proposal)
		}
		c.JSON(200, res)
	} else {
		// 执行节点
		bt1, bt2, bt3 := structure.PackTransactionBatch(height, int(shard))
		var res []structure.Proposal
		proposal := structure.Proposal{
			Shard:         shard,
			Height:        uint(height),
			InternalBatch: bt1,
			CrossBatch:    bt2,
			SuperBatch:    bt3,
		}
		res = append(res, proposal)
		c.JSON(200, res)
	}
}

// 共识分片中的节点通过该函数，请求将共识区块（即交易列表）上链
func AppendBlock(c *gin.Context) {
	var (
		leaderProblock      structure.ProposalBlock
		leaderProblockCount int
		OrderClientNum      = structure.ProposerNum
		leaderProblocks     = structure.Source.LeaderProblocksInfo
	)
	if err := c.ShouldBindJSON(&leaderProblock); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

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
			return
		}
		for i := 1; i <= structure.ShardNum; i++ {
			structure.Source.ChainShard[uint(0)].AccountState.RootsVote[uint(i)] = structure.Source.ChainShard[uint(0)].AccountState.NewRootsVote[uint(i)]
			structure.Source.ChainShard[uint(0)].AccountState.NewRootsVote[uint(i)] = make(map[string]int)
			structure.Source.ChainShard[uint(0)].NodeNum = structure.NewNodeNum()
			structure.Source.LeaderProblocksInfo = make(map[string]structure.LeaderProblockInfo)
			structure.Source.ClientShard[uint(i)].AccountState.Tx_num = 0
			structure.Source.ClientShard[uint(i)].AccountState.NewRootsVote = make(map[uint]map[string]int)
			structure.Source.ClientShard[uint(i)].ValidEnd = false
			for j := 1; j <= structure.ShardNum; j++ {
				structure.Source.ClientShard[uint(i)].AccountState.NewRootsVote[uint(j)] = make(map[string]int)
			}
		}
		res := model.BlockUploadResponse{
			Height:  uint(structure.GetHeight()),
			Message: "上传成功，并且proposal block已上链",
		}
		c.JSON(200, res)
	}
	res := model.BlockUploadResponse{
		Height:  uint(structure.GetHeight()),
		Message: "上传成功",
	}
	c.JSON(200, res)
}

// 检查是否已经达到 OrderClientNum 个 leaderProblock
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
		Block := structure.MakeBlock(maxProblockInfo.Block.ProposalList, uint(maxProblockInfo.Block.Height), maxProblockInfo.Block.Root)
		Block.Header.Vote = uint(maxCount)
		err := Chain.VerifyBlock(Block)
		if err != nil {
			logger.AnalysisLogger.Println("VerifyBlock Error: ", err)
			return err
		}
		err = Chain.AppendBlock(Block)
		if err != nil {
			return err
		}
	} else {
		return errors.New("收到投票数不足")
	}
	return nil
}

func PackAccount(c *gin.Context) {
	var data model.BlockAccountRequest
	if err := c.ShouldBindJSON(&data); err != nil {
		// gin.H封装了生成json数据的工具
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	shard := data.Shard
	accountList := structure.Source.ChainShard[0].AccountState.GetAccountList()
	gsroot := structure.GSRoot{
		StateRoot: structure.Source.ChainShard[0].AccountState.CalculateRoot(),
		Vote:      structure.Source.ChainShard[0].AccountState.RootsVote,
	}
	// logger.AnalysisLogger.Printf("来自分片%v的gsroot请求,此时rootsvote为:%v", shard, structure.Source.ChainShard[0].AccountState.RootsVote)
	height := structure.GetHeight()

	res := model.BlockAccountResponse{
		Shard:       shard,
		Height:      uint(height),
		AccountList: accountList,
		GSRoot:      gsroot,
	}
	// logger.AnalysisLogger.Println(res)
	c.JSON(200, res)
}

// MultiCastProposal 将proposalBlock路由转发给其他排序节点
func MultiCastProposal(c *gin.Context) {
	var data structure.ProposalBlock
	if err := c.ShouldBindJSON(&data); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	/*
		补充路由，可见controller/shard.go/MultiCastBlock
	*/

	payload, err := json.Marshal(data)
	if err != nil {
		log.Println(err)
		return
	}

	metaMessage := model.MessageMetaData{
		MessageType: 11,
		Message:     payload,
	}

	ComMap := structure.Source.ClientShard[0].Consensus_CommunicationMap
	for _, id := range data.IdList {
		client, err := ComMap.Get(id, 0)
		if err != nil {
			return
		}
		mu.Lock() // 加锁
		err = client.Socket.WriteJSON(metaMessage)
		mu.Unlock() // 解锁
		if err != nil {
			return
		}
	}

	res := model.MultiCastProposalResponse{
		Message: "Group multicast proposal succeed",
	}
	c.JSON(200, res)
}

// 生成 leaderProblock 的字符串标识
func GetProblockKey(leaderProblock structure.ProposalBlock) string {
	stringJSON, err := json.Marshal(leaderProblock)
	if err != nil {
		return ""
	}
	return string(stringJSON)
}

// func WitnessTx(c *gin.Context) {
// 	var data model.TxWitnessRequest_2
// 	//判断请求的结构体是否符合定义
// 	if err := c.ShouldBindJSON(&data); err != nil {
// 		// gin.H封装了生成json数据的工具
// 		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
// 		return
// 	}

// 	// shard := data.Shard

// 	//更新witnesstransDB中的witnessnum
// 	tx := structure.TransDB.Begin()
// 	txlist := []structure.Witnesstrans{}
// 	if err := tx.Where("id<?", 2*structure.TX_NUM+1).Find(&txlist).Error; err != nil {
// 		tx.Rollback()
// 		return
// 	}
// 	tx_temp := structure.Witnesstrans{
// 		WitnessNum: txlist[0].WitnessNum + 1,
// 	}
// 	if err := tx.Model(&txlist).Updates(tx_temp).Error; err != nil {
// 		tx.Rollback()
// 		return
// 	}
// 	tx.Commit()

// 	logger.AnalysisLogger.Printf("见证成功%v条交易", len(txlist))

// 	res := model.TxWitnessResponse_2{
// 		Message: "见证成功!",
// 		Flag:    true,
// 	}

// 	c.JSON(200, res)
// }

// func PackValidTx(c *gin.Context) {
// 	var data model.BlockTransactionRequest
// 	//判断请求的结构体是否符合定义
// 	if err := c.ShouldBindJSON(&data); err != nil {
// 		// gin.H封装了生成json数据的工具
// 		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
// 		return
// 	}
// 	shard := data.Shard

// 	height := structure.GetHeight() - 1
// 	IntList := make(map[uint][]structure.InternalTransaction)
// 	CroList := make(map[uint][]structure.CrossShardTransaction)
// 	ReList := make(map[uint][]structure.SuperTransaction)
// 	var Num int

// 	Int, Cro, Sup, num1, num2, num3 := structure.PackValidTransactionList(int(height)+1, int(shard))
// 	IntList[uint(shard)] = append(IntList[uint(shard)], Int...)
// 	CroList[uint(shard)] = append(CroList[uint(shard)], Cro...)
// 	ReList[uint(shard)] = append(ReList[uint(shard)], Sup...)

// 	logger.AnalysisLogger.Printf("执行节点验证阶段打包了%v,%v,%v条交易", num1, num2, num3)
// 	Num = num1 + num2 + num3
// 	res := model.BlockTransactionResponse{
// 		Shard:          shard,
// 		Height:         uint(structure.GetHeight() + 1),
// 		Num:            Num,
// 		InternalList:   IntList,
// 		CrossShardList: CroList,
// 		RelayList:      ReList,
// 	}

// 	c.JSON(200, res)
// }

// func CollectRoot(c *gin.Context) {
// 	var data model.RootUploadRequest
// 	//判断请求的结构体是否符合定义
// 	if err := c.ShouldBindJSON(&data); err != nil {
// 		// gin.H封装了生成json数据的工具
// 		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
// 		return
// 	}
// 	height := data.Height
// 	tx_num := data.TxNum
// 	root := data.Root
// 	shard := data.Shard
// 	sulist := data.SuList
// 	structure.Source.ClientShard[shard].Lock.Lock()

// 	structure.Source.ClientShard[uint(shard)].AccountState.NewRootsVote[shard][root] += 1
// 	logger.AnalysisLogger.Printf("收到来自shard%v的树根投票,该分片的树根以及票数情况为:votes%v", shard, structure.Source.ClientShard[uint(shard)].AccountState.NewRootsVote[shard])

// 	isEnd := structure.Source.ClientShard[shard].ValidEnd
// 	flag := false
// 	for _, num := range structure.Source.ClientShard[uint(shard)].AccountState.NewRootsVote[shard] {
// 		if num >= 1 {
// 			flag = true
// 		}
// 	}
// 	if !isEnd && flag {
// 		for _, tran := range sulist[shard] {
// 			traninDB := structure.Witnesstrans{
// 				Shard:      int(tran.FromShard),
// 				ToShard:    int(tran.Shard),
// 				FromAddr:   tran.From,
// 				ToAddr:     tran.To,
// 				TransValue: tran.Value,
// 				TransType:  2,
// 			}
// 			structure.AppendTransaction(traninDB)
// 		}
// 		logger.BlockLogger.Printf("分片%v写入%v条relaytx\n", shard, len(sulist[shard]))
// 		structure.Source.ClientShard[shard].AccountState.Tx_num += tx_num
// 		logger.BlockLogger.Printf("分片%v验证成功%v条交易\n", shard, tx_num)
// 		structure.Source.ClientShard[shard].ValidEnd = true
// 	}

// 	structure.Source.ClientShard[shard].Lock.Unlock()
// 	res := model.RootUploadResponse{
// 		Height:  height,
// 		Message: "树根上传成功",
// 	}
// 	c.JSON(200, res)
// }
