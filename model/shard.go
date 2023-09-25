package model

import (
	"server/logger"
	"server/structure"
)

type (
	ShardNumResponse struct {
		ShardNum uint
	}

	ConsensusFlagResponse struct {
		Flag bool
	}

	MessageMetaData struct {
		MessageType uint
		Message     []byte
	}

	// MessageReady MessageType = 0
	MessageReady struct {
		PersonalID string
		IdList     []string
	}

	// MessageIsWin MessageType = 1
	MessageIsWin struct {
		IsWin       bool
		IsConsensus bool
		WinID       string
		PersonalID  string
		IdList      []string
		ShardNum    int
	}

	//MessageType = 2
	MultiCastBlockRequest struct {
		Shard uint
		Id    string
		Block structure.Block
	}
	MultiCastBlockResponse struct {
		Message string
	}
	//MessageType = 3
	SendVoteRequest struct {
		Shard       uint
		BlockHeight int
		WinID       string
		PersonalID  string
		Agree       bool
	}
	SendVoteResponse struct {
		Message string
	}

	HeightResponse struct {
		Height int
	}
)

func PreRegister() ShardNumResponse {
	var BlockHeight int64
	// 读取区块高度
	tx2 := structure.ChainDB.Begin()
	if err := tx2.Model(&structure.Blocks{}).Count(&BlockHeight).Error; err != nil {
		return ShardNumResponse{
			ShardNum: uint(110),
		}
	}
	tx2.Commit()
	// 写入clientDB
	tx1 := structure.ClientDB.Begin()
	client := structure.Clients{
		Shard:      111,
		Height:     int(BlockHeight) + 1,
		ClientType: 0,
	}
	if err := tx1.Create(&client).Error; err != nil {
		tx1.Rollback()
		logger.AnalysisLogger.Printf("插入失败")
	}

	if err := tx1.Commit().Error; err != nil {
		tx1.Rollback()
		logger.AnalysisLogger.Printf("提交失败")
	}
	res := ShardNumResponse{
		ShardNum: uint(12138),
	}
	return res
}
