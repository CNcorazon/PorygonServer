package controller

import (
	"github.com/gin-gonic/gin"
	"net/http"
	"server/logger"
	"server/model"
	"server/structure"
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

	res = model.PackTransaction(data)
	c.JSON(200, res)
}

// AppendBlock 共识分片中的节点通过该函数，请求将共识区块（即交易列表）上链
func AppendBlock(c *gin.Context) {
	var (
		leaderProblock structure.ProposalBlock
		res            model.BlockUploadResponse
	)
	if err := c.ShouldBindJSON(&leaderProblock); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	err, isFinished := model.AppendBlock(leaderProblock)
	if err != nil {
		logger.AnalysisLogger.Println("上链失败：", err)
		res = model.BlockUploadResponse{
			Height:  uint(structure.GetHeight()),
			Message: "失败，请检查上链过程！",
		}
		return
	}
	if isFinished {
		res = model.BlockUploadResponse{
			Height:  uint(structure.GetHeight()),
			Message: "上传成功，并且proposal block已上链",
		}
		c.JSON(200, res)
	} else {
		res = model.BlockUploadResponse{
			Height:  uint(structure.GetHeight()),
			Message: "上传成功",
		}
		c.JSON(200, res)
	}
}

func PackAccount(c *gin.Context) {
	var data model.BlockAccountRequest
	if err := c.ShouldBindJSON(&data); err != nil {
		// gin.H封装了生成json数据的工具
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	res := model.PackAccount(data)
	c.JSON(200, res)
}

// MultiCastProposal 将proposalBlock路由转发给其他排序节点
//func MultiCastProposal(c *gin.Context) {
//
//	var data structure.ProposalBlock
//	if err := c.ShouldBindJSON(&data); err != nil {
//		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
//		return
//	}
//
//	/*
//		补充路由，可见controller/shard.go/MultiCastBlock
//	*/
//	payload, err := json.Marshal(data)
//	if err != nil {
//		log.Println(err)
//		return
//	}
//
//	metaMessage := model.MessageMetaData{
//		MessageType: 11,
//		Message:     payload,
//	}
//
//	ComMap := structure.Source.ClientShard[0].ConsensusCommunicationmap
//
//	logger.AnalysisLogger.Println(data.IdList)
//	for _, id := range data.IdList {
//		client, err := ComMap.Get(id, 0)
//		logger.AnalysisLogger.Println(client)
//		if err != nil {
//			return
//		}
//		mu.Lock() // 加锁
//		err = client.Socket.WriteJSON(metaMessage)
//		if err != nil {
//			return
//		}
//		mu.Unlock() // 解锁
//	}
//
//	res := model.MultiCastProposalResponse{
//		Message: "Group multicast proposal succeed",
//	}
//	c.JSON(200, res)
//}

func MultiCastProposal(c *gin.Context) {
	var data structure.ProposalBlock
	if err := c.ShouldBindJSON(&data); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	res := model.MuiltiCastProposal(data)
	c.JSON(http.StatusOK, res)
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

func PackValidTx(c *gin.Context) {
	var (
		data model.BlockTransactionRequest
	)
	if err := c.ShouldBindJSON(&data); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	res := model.PackValidTx(data)
	c.JSON(200, res)
}

func CollectRoot(c *gin.Context) {
	var data model.RootUploadRequest
	if err := c.ShouldBindJSON(&data); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	res := model.CollectRoot(data)
	c.JSON(200, res)
}

func GetProposalBlock(c *gin.Context) {
	var data model.GetProposalRequest
	if err := c.ShouldBindJSON(&data); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	res := model.GetProposalBlock(data)
	c.JSON(200, res)
}
