package request

import (
	"log"
	"math/rand"
	"time"

	"github.com/gorilla/websocket"
)

func WSRequest(wsurl string, route string, ip string) (*websocket.Conn, bool) {
	rand.Seed(time.Now().UnixNano())
	URL := wsurl + route + "/" + ip
	var dialer *websocket.Dialer
	conn, _, err := dialer.Dial(URL, nil)
	if err != nil {
		log.Printf("WS连接已经断开,等待服务器开始")
		return nil, false
	}
	log.Printf("WS连接成功")
	return conn, true
}

//func ClientRegister(httpurl string, route string, clientdata model.ClientForwardRequest) model.ClientForwardResponse {
//	URL := httpurl + route
//	data := clientdata
//	jsonData, err := json.Marshal(data)
//	if err != nil {
//		log.Println(err)
//	}
//	request, _ := http.NewRequest("POST", URL, bytes.NewBuffer(jsonData))
//	request.Header.Set("Content-Type", "application/json")
//	client := &http.Client{}
//	response, err := client.Do(request)
//	if err != nil {
//		panic(err)
//	}
//	body, _ := ioutil.ReadAll(response.Body)
//	var res model.ClientForwardResponse
//	json.Unmarshal(body, &res)
//	return res
//}

// func SynTxpool(httpurl string, route string)
