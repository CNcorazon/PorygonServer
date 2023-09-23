package structure

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"server/logger"
	"sync"
	"time"
)

type (
	HorizonBlockChain struct {
		NodeNum      *NodeNum
		Height       uint    // 当前区块链的高度
		Chain        []Block //被添加到链上的区块
		AccountState *State  //当前区块链的状态
		TotalTxNum   int
		InitTime     int64
	}

	NodeNum struct {
		Nodeshard map[int]int
		Mu        sync.Mutex
	}

	Block struct {
		Header BlockHeader
		Body   BlockBody
	}

	Proposal struct {
		Shard         uint
		Height        uint
		InternalBatch []Batchtrans
		CrossBatch    []Batchtrans
		SuperBatch    []Batchtrans
	}

	// ProposalBlock 共识阶段：排序委员会提出的proposal block
	ProposalBlock struct {
		Id            string
		IdList        []string
		LockedAccount []int
		Height        int
		Hash          string     //前一个区块的hash
		Root          GSRoot     //状态树树根
		ProposalList  []Proposal //交易列表
		Vrf           int        //vrf
	}

	BlockHeader struct {
		// Shard                uint   //表示是第几号分片中的区块
		Height          uint   //当前区块的高度
		Time            int64  //区块产生的时候的Unix时间戳
		Vote            uint   //本区块收到的移动节点的票数
		TransactionRoot string //修改了本分片状态的交易区块的SHA256值
		// SuperTransactionRoot string //产生的超级交易区块的SHA256值
		StateRoot GSRoot //当前执行完本交易之后，当前区块链账本的世界状态 //应该是前一个时间的状态！
	}

	BlockBody struct {
		// Shard            uint
		Height           uint
		LockedAccount    []int
		TransactionLists []Proposal
		// SuperTransaction SuperTransactionBlock
	}

	GSRoot struct {
		StateRoot string
		Vote      map[uint]map[string]int //记录每个执行分片计算出的subTreeRoot以及对应的票数
	}

	// AllTransactionBlock struct {
	// 	AllTransaction []TransactionBlock
	// }

	TransactionBlock struct {
		// Id             string
		Height         uint
		InternalList   map[uint][]InternalTransaction
		CrossShardList map[uint][]CrossShardTransaction
		SuperList      map[uint][]SuperTransaction //需要被打包进这个区块内部的SuperList
		Sign           PubKeySign
	}

	SuperTransactionBlock struct {
		SuperTransaction map[uint][]SuperTransaction //执行完成TransactionList之后生成的一个ReplayList
	}
)

//func (r *TransactionBlock) CalculateRoot() string {
//	jsonString, err := json.Marshal(r)
//	if err != nil {
//		log.Fatalln("计算交易区块Root失败")
//	}
//	byte32 := sha256.Sum256(jsonString)
//	return hex.EncodeToString(byte32[:])
//}

func MakeHorizonBlockChain(n int, shardNum int) *HorizonBlockChain {
	state := InitState(n, shardNum)
	state.InitAccountList(shardNum, n)
	start := time.Now().UnixMicro()
	chain := HorizonBlockChain{
		Height:       0,
		NodeNum:      NewNodeNum(),
		Chain:        make([]Block, 0),
		AccountState: state,
		TotalTxNum:   0,
		InitTime:     start,
	}
	return &chain
}

func NewNodeNum() *NodeNum {
	m := make(map[int]int)
	for i := 1; i <= ShardNum; i++ {
		m[i] = 0
	}
	N := NodeNum{
		Nodeshard: m,
		Mu:        sync.Mutex{},
	}
	return &N
}

// VerifyBlock 验证共识区块是否应该被添加到链上
func (a *HorizonBlockChain) VerifyBlock(b Block) error {
	//检测区块序列号是否符合
	MinVote := math.Max(0, math.Floor(2*float64(ProposerNum/3)))
	logger.AnalysisLogger.Println(b.Header.Height, GetHeight()+1)
	if b.Header.Height != uint(GetHeight())+1 {
		return errors.New("区块的高度不符合")
	} else if b.Header.Vote < uint(MinVote) {
		return errors.New("区块没有收集到足够多的投票")
	} else {
		return nil
	}
}

func (a *HorizonBlockChain) AppendBlock(b Block) error {
	tx := ChainDB.Begin()

	jsonData1, err := json.Marshal(b.Header.TransactionRoot)
	if err != nil {
		// 处理错误
		fmt.Println(err)
		return err
	}
	jsonData2, err := json.Marshal(b.Body.TransactionLists)
	if err != nil {
		// 处理错误
		fmt.Println(err)
		return err
	}
	jsonData3, err := json.Marshal(b.Header.StateRoot.StateRoot)
	if err != nil {
		// 处理错误
		fmt.Println(err)
		return err
	}
	block := Blocks{
		Height:           int(b.Header.Height),
		Timeonchain:      int(b.Header.Time),
		Vote:             int(b.Header.Vote),
		TransactionRoot:  string(jsonData1),
		TransactionRoots: string(jsonData2),
		StateRoot:        string(jsonData3),
	}
	if err := tx.Create(&block).Error; err != nil {
		log.Println(err)
		tx.Rollback()
	}
	tx.Commit()
	// 更新账户状态
	UpdateChainWithBlock(b, a.AccountState)
	return nil
}

func GetHeight() int {
	latestBlock := Blocks{}
	tx := ChainDB.Begin()
	tx.Order("Height DESC").Limit(1).Find(&latestBlock)
	tx.Commit()
	height := latestBlock.Height
	return height
}

func MakeBlock(problock ProposalBlock, height uint, gsroot GSRoot) Block {
	body := BlockBody{
		// Shard:            s.Shard,
		Height:           height,
		LockedAccount:    problock.LockedAccount,
		TransactionLists: problock.ProposalList,
		// SuperTransaction: SuperBlock,
	}

	root, _ := json.Marshal(problock.LockedAccount)
	byte32 := sha256.Sum256(root)
	tr := hex.EncodeToString(byte32[:])
	header := BlockHeader{
		// Shard:                s.Shard,
		Height:          height,
		Time:            time.Now().Unix(),
		Vote:            1,
		TransactionRoot: tr,
		StateRoot:       gsroot,
	}

	block := Block{
		Header: header,
		Body:   body,
	}

	return block
}
