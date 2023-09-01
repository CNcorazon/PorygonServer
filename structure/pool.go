package structure

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log"
	"server/logger"
)

//从witnessDB中打包需要处理的交易
//加上redis缓存，这样每轮的区块见证就只需要从mysql中读取一次数据

/*
func PackTransactionList(height, shard, num1, num2 int) ([]InternalTransaction, []CrossShardTransaction, int) {
	translist := []Witnesstrans{}
	tx := WitnessTransDB.Begin()
	var InterTransList []InternalTransaction
	var CrossTransList []CrossShardTransaction
	//只读最前面的交易，模拟（可以加上offset=height*num1
	if err := tx.Where("shard = ? AND trans_type = 0", shard).Limit(num1).Find(&translist).Error; err != nil {
		log.Println(err)
		tx.Rollback()
		return []InternalTransaction{}, []CrossShardTransaction{}, 0
	} else {
		for _, rawtrans := range translist {
			tran := InternalTransaction{
				Shard: uint(rawtrans.Shard),
				From:  rawtrans.FromAddr,
				To:    rawtrans.ToAddr,
				Value: rawtrans.TransValue,
			}
			InterTransList = append(InterTransList, tran)
		}
	}

	if err := tx.Where("shard = ? AND trans_type=1", shard).Limit(num1).Find(&translist).Error; err != nil {
		tx.Rollback()
		return []InternalTransaction{}, []CrossShardTransaction{}, 0
	} else {
		for _, rawtrans := range translist {
			tran := CrossShardTransaction{
				Shard1: uint(rawtrans.Shard),
				Shard2: uint(rawtrans.ToShard),
				From:   rawtrans.FromAddr,
				To:     rawtrans.ToAddr,
				Value:  rawtrans.TransValue,
			}
			CrossTransList = append(CrossTransList, tran)
		}
	}

	tx.Commit()
	return InterTransList, CrossTransList, len(InterTransList) + len(CrossTransList)
}*/

// 打包transactionbatch
func PackTransactionBatch(height, shard int) ([]TransactionBatch, []TransactionBatch, []TransactionBatch) {
	var (
		bts              []Batchtrans
		BatchTransaction = make(map[int][]TransactionBatch)
	)
	//bts := []Batchtrans{}
	//BatchTransaction := make(map[int][]TransactionBatch, 0)

	for i := 0; i <= 2; i++ {
		if err := TransDB.Where("trans_type = ? AND shard = ?", i, shard).Limit(TX_NUM / 2000).Find(&bts).Error; err != nil {
			logger.AnalysisLogger.Printf("分片%v中没有batch内部交易！", shard)
		}
		// 用于解压缩公钥索引字段和签名字段
		for _, bt := range bts {
			BT := TransactionBatch{
				Shard:    uint(bt.Shard),
				Abstract: bt.Abstract,
				// PubIndex: ,
				// Sig: ,
			}
			if BatchTransaction[i] == nil {
				BatchTransaction[i] = make([]TransactionBatch, 0)
			}
			BatchTransaction[i] = append(BatchTransaction[i], BT)
		}
	}
	return BatchTransaction[0], BatchTransaction[1], BatchTransaction[2]
}

// 打包witness超过一定阈值的交易
func PackWitnessedTransactionList(s *State, height, shard int) ([]InternalTransaction, []CrossShardTransaction, []SuperTransaction, int) {
	var Num int
	InternalList := []Witnesstrans{}
	CrossShardList := []Witnesstrans{}
	SuperList := []Witnesstrans{}
	var InterTransList []InternalTransaction
	var CrossTransList []CrossShardTransaction
	var SuperTransList []SuperTransaction

	tx := TransDB.Begin()
	if err := tx.Where("witness_num > ? AND trans_type = 1 AND shard = ?", CLIENT_MAX*2/3, shard).Limit(TX_NUM).Find(&CrossShardList).Error; err != nil {
		logger.AnalysisLogger.Printf("分片%v中没有witnessed跨分片交易！", shard)
		tx.Commit()
		return InterTransList, CrossTransList, SuperTransList, Num
	}
	for _, rawtrans := range CrossShardList {
		tran := CrossShardTransaction{
			Shard1: uint(rawtrans.Shard),
			Shard2: uint(rawtrans.ToShard),
			From:   rawtrans.FromAddr,
			To:     rawtrans.ToAddr,
			Value:  rawtrans.TransValue,
		}
		// 执行完成后，将shard中的fromaddr账户解锁
		if LockAccount(s, rawtrans.Shard, rawtrans.FromAddr) && LockAccount(s, rawtrans.ToShard, rawtrans.ToAddr) {
			CrossTransList = append(CrossTransList, tran)
		}
	}

	if err := tx.Where("witness_num > ? AND trans_type = 0 AND shard = ?", CLIENT_MAX*2/3, shard).Limit(TX_NUM).Find(&InternalList).Error; err != nil {
		logger.AnalysisLogger.Printf("分片%v中没有witnessed内部交易！", shard)
		tx.Commit()
		return InterTransList, CrossTransList, SuperTransList, Num
	}
	for _, rawtrans := range InternalList {
		tran := InternalTransaction{
			Shard: uint(rawtrans.Shard),
			From:  rawtrans.FromAddr,
			To:    rawtrans.ToAddr,
			Value: rawtrans.TransValue,
		}
		//执行完成后解锁
		if LockAccount(s, rawtrans.Shard, rawtrans.FromAddr) && LockAccount(s, rawtrans.Shard, rawtrans.ToAddr) {
			InterTransList = append(InterTransList, tran)
		}
	}

	if err := tx.Where("witness_num > ? AND trans_type = 2 AND shard = ?", CLIENT_MAX*2/3, shard).Limit(TX_NUM).Find(&SuperList).Error; err != nil {
		logger.AnalysisLogger.Printf("分片%v中没有超级交易！", shard)
		tx.Commit()
		return InterTransList, CrossTransList, SuperTransList, Num
	}
	for _, rawtrans := range SuperList {
		tran := SuperTransaction{
			FromShard: uint(rawtrans.Shard),
			Shard:     uint(rawtrans.ToShard),
			From:      rawtrans.FromAddr,
			To:        rawtrans.ToAddr,
			Value:     rawtrans.TransValue,
		}
		//无需加锁，用于解锁
		SuperTransList = append(SuperTransList, tran)

	}
	tx.Commit()
	Num += len(InterTransList) + len(CrossTransList) + len(SuperTransList)
	return InterTransList, CrossTransList, SuperTransList, Num
}

func PackValidTransactionList(height, shard int) ([]InternalTransaction, []CrossShardTransaction, []SuperTransaction, int, int, int) {
	translist := []Witnesstrans{}
	tx := TransDB.Begin()
	var InterTransList []InternalTransaction
	var CrossTransList []CrossShardTransaction
	var SuperTransList []SuperTransaction
	//只读最前面的交易，模拟（可以加上offset=height*num1
	if err := tx.Where("shard = ? AND trans_type=0", shard).Limit(TX_NUM).Find(&translist).Error; err != nil {
		tx.Rollback()
		return []InternalTransaction{}, []CrossShardTransaction{}, []SuperTransaction{}, 0, 0, 0
	} else {
		for _, rawtrans := range translist {
			tran := InternalTransaction{
				Shard: uint(rawtrans.Shard),
				From:  rawtrans.FromAddr,
				To:    rawtrans.ToAddr,
				Value: rawtrans.TransValue,
			}
			InterTransList = append(InterTransList, tran)
		}
	}

	if err := tx.Where("shard = ? AND trans_type=1", shard).Limit(TX_NUM).Find(&translist).Error; err != nil {
		tx.Rollback()
		return []InternalTransaction{}, []CrossShardTransaction{}, []SuperTransaction{}, 0, 0, 0
	} else {
		for _, rawtrans := range translist {
			tran := CrossShardTransaction{
				Shard1: uint(rawtrans.Shard),
				Shard2: uint(rawtrans.ToShard),
				From:   rawtrans.FromAddr,
				To:     rawtrans.ToAddr,
				Value:  rawtrans.TransValue,
			}
			CrossTransList = append(CrossTransList, tran)
		}
	}

	if err := tx.Where("shard = ? AND trans_type=2", shard).Limit(TX_NUM).Find(&translist).Error; err != nil {
		tx.Rollback()
		return []InternalTransaction{}, []CrossShardTransaction{}, []SuperTransaction{}, 0, 0, 0
	} else {
		for _, rawtrans := range translist {
			tran := SuperTransaction{
				FromShard: uint(rawtrans.Shard),
				Shard:     uint(rawtrans.ToShard),
				From:      rawtrans.FromAddr,
				To:        rawtrans.ToAddr,
				Value:     rawtrans.TransValue,
			}
			SuperTransList = append(SuperTransList, tran)
		}
	}

	tx.Commit()
	return InterTransList, CrossTransList, SuperTransList, len(InterTransList), len(CrossTransList), len(SuperTransList)
}

func AppendTransaction(trans Witnesstrans) error {
	//写进数据库
	tx := TransDB.Begin()
	if err := tx.Create(&trans).Error; err != nil {
		tx.Rollback()
		return err
	}
	tx.Commit()
	return nil
}

func AppendTransactionBatch(translist []Witnesstrans, shard int, trantype int) error {
	jsonByte, _ := json.Marshal(translist)
	byte32 := sha256.Sum256(jsonByte)
	Abstring := hex.EncodeToString(byte32[:])
	tb := Batchtrans{
		Shard:     shard,
		Abstract:  Abstring,
		TransType: trantype,
	}
	if err := TransDB.Create(&tb).Error; err != nil {
		return err
	}
	return nil
}

func LockAccount(s *State, s1 int, addr1 string) bool {
	_, flag := s.AccountMap[uint(s1)][addr1]
	if !flag {
		log.Fatalf("加锁阶段: 分片%v该节点%v地址不存在！", s1, addr1)
	}
	if !TryLock(s.AccountMap[uint(s1)][addr1]) {
		return false
	}
	return true
}
