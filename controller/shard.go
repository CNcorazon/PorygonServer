package controller

import (
	"encoding/json"
	"errors"
	"github.com/gorilla/websocket"
	"gorm.io/gorm/clause"
	"log"
	"net/http"
	"server/logger"
	"server/model"
	"server/structure"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	uuid "github.com/satori/go.uuid"
	"gorm.io/gorm"
)

// const (
// 	WsRequest     = "/forward/wsRequest"
// 	ClientForward = "/forward/clientRegister "
// )

// PreRegister OrderClient登录进数据库（节点预登记）
func PreRegister(c *gin.Context) {
	res := model.PreRegister()
	c.JSON(200, res)
}

// RegisterCommunication OrderClients建立websocket
func RegisterCommunication(c *gin.Context) {
	var (
		ID              string
		clientCount     int64
		clients         []structure.Clients
		orderClientsNum = structure.ProposerNum
		h               = structure.GetHeight() + 1
	)

	//if err := structure.ClientDB.Exec("SET SESSION TRANSACTION ISOLATION LEVEL SERIALIZABLE").Error; err != nil {
	//	log.Printf("设置隔离级别失败: %v", err)
	//	return
	//}

	// 在事务外部进行查询: 高度为h的区块中排序节点的数量
	if err := structure.ClientDB.Model(&structure.Clients{}).Where("shard = ? AND height = ?", 0, h).Count(&clientCount).Error; err != nil {
		// 处理查询错误
		log.Println(err)
		return
	}
	logger.AnalysisLogger.Println(clientCount)
	if int(clientCount) >= orderClientsNum {
		logger.AnalysisLogger.Println("排序节点已满！")
		return
	}

	// 若未满，则建立websocket，并记录在ClientDB和内存中
	logger.ShardLogger.Printf("该排序节点被分配到了第%v个区块", h)
	// 创建事务
	tx := structure.ClientDB.Begin()
	// 更新ClientDB
	orderShardNum := 0
	maxRetries := 3
	for i := 0; i < maxRetries; i++ {
		err := structure.ClientDB.Transaction(func(tx *gorm.DB) error {
			clientInDB := structure.Clients{}
			id := uuid.NewV4().String()
			// 锁定记录
			result := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
				Where("shard = ? AND height = ?", 111, h).
				First(&clientInDB)
			// 更新找到的记录
			clientInDB.Shard = 0
			clientInDB.Pubkey = id
			clientInDB.ClientType = 1
			if result.Error == nil {
				if err := tx.Save(&clientInDB).Error; err != nil {
					tx.Rollback()
					return err
				}
			}
			if errors.Is(result.Error, gorm.ErrRecordNotFound) {
				// 如果没有找到记录，创建一个新的记录
				log.Printf("未找到记录，新建一条记录")
				newClient := structure.Clients{
					Shard:      0,
					Pubkey:     id,
					ClientType: 1,
					Height:     structure.GetHeight() + 1,
				}
				if err := tx.Create(&newClient).Error; err != nil {
					tx.Rollback()
					return err
				}
			} else if result.Error != nil {
				tx.Rollback()
				return result.Error
			}
			ID = id
			// 如果操作成功，返回 nil，事务将会被提交
			return nil
		})
		if err == nil {
			// log.Println()
			break
		}
		if strings.Contains(err.Error(), "Deadlock found") && i < maxRetries-1 {
			log.Printf("死锁检测到，重试事务（尝试 %d/%d）", i+1, maxRetries)
		} else {
			log.Printf("事务失败: %v,重试事务，尝试 %d/%d）", err, i+1, maxRetries)
		}
		time.Sleep(50 * time.Millisecond)
	}
	//将http请求升级成为WebSocket请求
	upGrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
		Subprotocols: []string{c.GetHeader("Sec-WebSocket-Protocol")},
	}
	conn, err := upGrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("websocket connect error: %s", c.Param("channel"))
		return
	}
	client := &structure.MapClient{
		Id:     ID,
		Shard:  uint(orderShardNum),
		Socket: conn,
	}

	//var clients []structure.Clients

	tx.Where("shard = ? AND client_type = 1 AND height = ? ", orderShardNum, h).Find(&clients)
	logger.ShardLogger.Printf("排序委员会中有%v个节点", len(clients))
	tx.Commit()

	ConsensusMap := structure.Source.ClientShard[0].ConsensusCommunicationmap
	ConsensusMap.Put(client.Id, client)

	// 委员会满了，通知开始共识
	if len(clients) == orderClientsNum {
		logger.ShardLogger.Printf("排序委员会已满，通知排序节点开始共识")
		var idList []string
		for _, clientInDB := range clients {
			client, err := structure.Source.ClientShard[0].ConsensusCommunicationmap.Get(clientInDB.Pubkey, 0)
			if err != nil {
				logger.AnalysisLogger.Printf("找不到client:%v", clientInDB.Pubkey)
				return
			}
			idList = append(idList, client.Id)
		}
		//通知排序委员会开始共识
		for _, clientInDB := range clients {
			client, err := structure.Source.ClientShard[0].ConsensusCommunicationmap.Get(clientInDB.Pubkey, 0)
			if err != nil {
				logger.AnalysisLogger.Printf("找不到client:%v", clientInDB.Pubkey)
				return
			}
			message := model.MessageReady{
				PersonalID: client.Id,
				IdList:     idList,
			}
			payload, err := json.Marshal(message)
			if err != nil {
				log.Println(err)
				return
			}
			metaMessage := model.MessageMetaData{
				MessageType: 0,
				Message:     payload,
			}
			err = client.Socket.WriteJSON(metaMessage)
			if err != nil {
				return
			}
		}
	}
}

func GetHeight(c *gin.Context) {
	res := model.HeightResponse{
		Height: structure.GetHeight(),
	}
	c.JSON(200, res)
}

// MultiCastBlock 共识分片内部的胜利者计算出区块之后，使用该函数向分片内部的节点转发计算出来得到的区块
func MultiCastBlock(c *gin.Context) {
	var data model.MultiCastBlockRequest
	//判断请求的结构体是否符合定义
	if err := c.ShouldBindJSON(&data); err != nil {
		// gin.H封装了生成json数据的工具
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// payload, err := json.Marshal(data)
	// if err != nil {
	// 	log.Println(err)
	// 	return
	// }
	// logger.TimestampLogger.Printf("广播的区块大小为%v", unsafe.Sizeof(payload))
	// metaMessage := model.MessageMetaData{
	// 	MessageType: 2,
	// 	Message:     payload,
	// }

	// var CommunicationMap map[uint]map[string]*structure.Client
	// if len(structure.Source.CommunicationMap[uint(0)]) > len(structure.Source.CommunicationMap_temp[uint(0)]) {
	// 	CommunicationMap = structure.Source.CommunicationMap
	// } else {
	// 	CommunicationMap = structure.Source.CommunicationMap_temp
	// }

	//向连接在该服务器的委员会成员转发数据，注意不用向自己转发

	// for key, value := range structure.Source.ClientShard[0].Consensus_CommunicationMap {
	// 	if key != data.Id && value.Socket != nil {
	// 		logger.AnalysisLogger.Printf("将区块发送给委员会成员")
	// 		value.Socket.WriteJSON(metaMessage)
	// 	}
	// }

	res := model.MultiCastBlockResponse{
		Message: "Group multicast block succeed",
	}
	c.JSON(200, res)
}

func SendVote(c *gin.Context) {
	// start := time.Now()
	var data model.SendVoteRequest
	//判断请求的结构体是否符合定义
	if err := c.ShouldBindJSON(&data); err != nil {
		// gin.H封装了生成json数据的工具
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// // structure.Source.ClientShard[0].NodeNum = structure.Source.ClientShard[0].NodeNum + 1

	// // shardnum := data.Shard
	// target := data.WinID
	// // log.Println(target)
	// // log.Println(shardnum)
	// payload, err := json.Marshal(data)
	// if err != nil {
	// 	log.Println(err)
	// 	return
	// }
	// metaMessage := model.MessageMetaData{
	// 	MessageType: 3,
	// 	Message:     payload,
	// }
	// structure.Source.ClientShard[data.Shard].Lock.Lock()
	// // structure.Source.ClientShard[0].Lock.Lock()
	// //logger.AnalysisLogger.Println(structure.Source.ClientShard[data.Shard].MultiCastConn)

	// if structure.Source.ClientShard[data.Shard].MultiCastConn != nil {
	// 	//logger.AnalysisLogger.Println(structure.Source.ClientShard[0].Consensus_CommunicationMap[target])
	// 	if structure.Source.ClientShard[0].Consensus_CommunicationMap[target].Socket != nil {
	// 		structure.Source.ClientShard[data.Shard].MultiCastConn.WriteJSON(metaMessage)
	// 	} else {
	// 		for _, value := range structure.Source.Write_Server_CommunicationMap {
	// 			value.Lock.Lock()
	// 			value.Socket.WriteJSON(metaMessage)
	// 			value.Lock.Unlock()
	// 		}
	// 	}
	// }
	// // time := time.Since(start)
	// // logger.AnalysisLogger.Println(time)
	// // structure.Source.ClientShard[data.Shard].Lock.Unlock()
	// structure.Source.ClientShard[0].Lock.Unlock()
	// // }

	res := model.SendVoteResponse{
		Message: "Group multicast Vote succeed",
	}
	c.JSON(200, res)
}

// func MuiltiCastCommunication(c *gin.Context) {
// 	shardnum, _ := strconv.Atoi(c.Param("shardnum"))

// 	//将http请求升级成为WebSocket请求
// 	upGrader := websocket.Upgrader{
// 		// cross origin domain
// 		CheckOrigin: func(r *http.request) bool {
// 			return true
// 		},
// 		// 处理 Sec-WebSocket-Protocol Header
// 		Subprotocols: []string{c.GetHeader("Sec-WebSocket-Protocol")},
// 	}

// 	conn, err := upGrader.Upgrade(c.Writer, c.request, nil)
// 	if err != nil {
// 		log.Printf("websocket connect error: %s", c.Param("channel"))
// 		return
// 	}

// 	structure.Source.ClientShard[uint(shardnum)].MultiCastConn = conn
// 	//转发给其他服务器
// }

// func stringInSlice(a string, list []string) bool {
// 	for _, b := range list {
// 		if b == a {
// 			return true
// 		}
// 	}
// 	return false
// }

// clientinDB.Shard = shardnum
// clientinDB.Pubkey = client.Id
// clientinDB.ClientType = 1
// clientinDB.Random = client.Random
// clientinDB.Height = h

// func updateClient(db *gorm.DB, clientinDB *structure.Clients, shardnum int, Pubkey string, client_type int, random int, h int) error {
// 	// 重试次数
// 	retryCount := 3
// 	var err error

// 	for i := 0; i < retryCount; i++ {
// 		err = db.Transaction(func(tx *gorm.DB) error {
// 			// 更新用户
// 			clientinDB.Shard = shardnum
// 			clientinDB.Pubkey = Pubkey
// 			clientinDB.ClientType = 1
// 			clientinDB.Random = random
// 			clientinDB.Height = h
// 			result := tx.Save(clientinDB)
// 			if result.Error != nil {
// 				return result.Error
// 			}

// 			return nil
// 		})

// 		// 如果事务成功，跳出循环
// 		if err == nil {
// 			break
// 		}
// 	}
// 	// 等待一段时间再次尝试
// 	time.Sleep(50 * time.Millisecond)

// 	return err
// }
