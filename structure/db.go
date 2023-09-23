package structure

type Blocks struct {
	Height           int    `gorm:"primaryKey"` //当前区块的高度
	Timeonchain      int    //区块产生的时候的Unix时间戳
	Vote             int    //本区块收到的移动节点的票数
	TransactionRoot  string //修改了本分片状态的交易区块的SHA256值
	StateRoot        string //当前执行完本交易之后，当前区块链账本的世界状态 //数据库中转化为JSON进行存储
	TransactionRoots string
}

type Clients struct {
	Id         int `gorm:"primaryKey"`
	Shard      int `gorm:"column:shard"`
	Height     int
	Pubkey     string `gorm:"default:' '"`
	ClientType int    // 0 for reshard waiting, 1 for rightnow
}

type Witnesstrans struct {
	Id         int `gorm:"primaryKey"` //作为主键索引
	Shard      int
	ToShard    int
	TransValue int
	FromAddr   string
	ToAddr     string
	TransType  int //0 for internal, 1 for external, 2 for super
	WitnessNum int //将超级交易的witness设置为MAX，排序后优先输出
}

type Batchtrans struct {
	Id             int
	Shard          int
	Abstract       string
	RelatedAccount []int // 第一维是内部交易的，第二维是跨分片from方的，第三维是跨分片to方的
	PubIndex       []int
	Sig            string
	TransType      int
}

// func (User) TableName() string {
// 	return "user"
// }
