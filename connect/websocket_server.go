package connect

import (
	"errors"
	"fmt"
	"github.com/golang/protobuf/proto"
	"github.com/gorilla/websocket"
	"goim/conf"
	"goim/public/lib"
	"goim/public/logger"
	"goim/public/pb"
	"goim/public/transfer"
	"log"
	"net/http"
	"sync"
	"time"
)

const (
	// 允许等待的写入时间
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer.
	pongWait = 60 * time.Second

	// Send pings to peer with this period. Must be less than pongWait.
	pingPeriod = (pongWait * 9) / 10

	// Maximum message size allowed from peer.
	maxMessageSize = 512
)

// 最大的连接ID，每次连接都加1 处理
var maxConnId int64

// 客户端读写消息
type wsMessage struct {
	// websocket.TextMessage 消息类型
	messageType int
	data        []byte
}

// ws 的所有连接
// 用于广播
var wsConnAll map[int64]*wsConnection

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	// 允许所有的CORS 跨域请求，正式环境可以关闭
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// 客户端连接
type wsConnection struct {
	wsSocket *websocket.Conn // 底层websocket
	inChan   chan *wsMessage // 读队列
	outChan  chan *wsMessage // 写队列

	mutex     sync.Mutex // 避免重复关闭管道,加锁处理
	isClosed  bool
	closeChan chan byte // 关闭通知
	id        int64
	IsSignIn  bool  // 是否登录
	DeviceId  int64 // 设备id
	UserId    int64 // 用户id
}

func wsHandler(resp http.ResponseWriter, req *http.Request) {
	// 应答客户端告知升级连接为websocket
	wsSocket, err := upgrader.Upgrade(resp, req, nil)
	if err != nil {
		log.Println("升级为websocket失败", err.Error())
		return
	}
	maxConnId++
	// TODO 如果要控制连接数可以计算，wsConnAll长度
	// 连接数保持一定数量，超过的部分不提供服务
	wsConn := &wsConnection{
		wsSocket:  wsSocket,
		inChan:    make(chan *wsMessage, 1000),
		outChan:   make(chan *wsMessage, 1000),
		closeChan: make(chan byte),
		isClosed:  false,
		id:        maxConnId,
	}
	wsConnAll[maxConnId] = wsConn
	log.Println("当前在线人数", len(wsConnAll))

	// 消息处理器
	go wsConn.processLoop()
	// 读协程
	go wsConn.wsReadLoop()
	// 写协程
	go wsConn.wsWriteLoop()
}

// 处理队列中的消息
func (wsConn *wsConnection) processLoop() {
	// 处理消息队列中的消息
	// 获取到消息队列中的消息，并调用处理逻辑
	for {
		msg, err := wsConn.wsRead()
		if err != nil {
			log.Println("获取消息出现错误", err.Error())
			break
		}
		protoMsgID := int(msg.data[LenLen-1])
		protoMsgContent := msg.data[HeadLen:]
		pack := &Package{Code: protoMsgID, Content: protoMsgContent}
		wsConn.HandlePackage(pack)
	}
}

// 处理消息队列中的消息
func (wsConn *wsConnection) wsReadLoop() {
	// 设置消息的最大长度
	wsConn.wsSocket.SetReadLimit(maxMessageSize)
	wsConn.wsSocket.SetReadDeadline(time.Now().Add(pongWait))
	wsConn.wsSocket.SetPongHandler(func(string) error { wsConn.wsSocket.SetReadDeadline(time.Now().Add(pongWait)); return nil })
	for {
		// 读一个message
		msgType, data, err := wsConn.wsSocket.ReadMessage()
		if err != nil {
			//websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure)
			log.Println("消息读取出现错误", err.Error())
			wsConn.close()
			return
		}
		req := &wsMessage{
			msgType,
			data,
		}
		// 放入请求队列,消息入栈
		select {
		case wsConn.inChan <- req:
		case <-wsConn.closeChan:
			return
		}
	}
}

// 发送消息给客户端
func (wsConn *wsConnection) wsWriteLoop() {
	//发送定时信息，避免意外关闭
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
	}()
	for {
		select {
		// 取一个应答
		case msg := <-wsConn.outChan:
			wsConn.wsSocket.SetWriteDeadline(time.Now().Add(writeWait))
			// 写给websocket
			if err := wsConn.wsSocket.WriteMessage(msg.messageType, msg.data); err != nil {
				log.Println("发送消息给客户端发生错误", err.Error())
				// 切断服务
				wsConn.close()
				return
			}
		case <-wsConn.closeChan:
			// 获取到关闭通知
			return
		case <-ticker.C:
			//出现超时情况
			wsConn.wsSocket.SetWriteDeadline(time.Now().Add(writeWait))
			if err := wsConn.wsSocket.WriteMessage(websocket.PingMessage, nil); err != nil {
				log.Println("发送消息给客户端发生错误", err.Error())
				return
			}
		}
	}
}

// 写入消息到队列中
func (wsConn *wsConnection) wsWrite(messageType int, data []byte) error {
	select {
	case wsConn.outChan <- &wsMessage{messageType, data}:
	case <-wsConn.closeChan:
		return errors.New("连接已经关闭")
	}
	return nil
}

// 读取消息队列中的消息
func (wsConn *wsConnection) wsRead() (*wsMessage, error) {
	select {
	case msg := <-wsConn.inChan:
		// 获取到消息队列中的消息
		return msg, nil
	case <-wsConn.closeChan:

	}
	return nil, errors.New("连接已经关闭")
}

// 关闭连接
func (wsConn *wsConnection) close() {
	log.Println("关闭连接被调用了")
	wsConn.wsSocket.Close()
	wsConn.mutex.Lock()
	defer wsConn.mutex.Unlock()
	if wsConn.isClosed == false {
		wsConn.isClosed = true
		// 删除这个连接的变量
		delete(wsConnAll, wsConn.id)
		log.Println("当前在线人数", len(wsConnAll))
		myWSDelete(wsConn.DeviceId)
		close(wsConn.closeChan)
		publishOffLine(transfer.OffLine{
			DeviceId: wsConn.DeviceId,
			UserId:   wsConn.UserId,
		})
	}
}

// 启动程序
func StartWebsocket(addrPort string) {
	wsConnAll = make(map[int64]*wsConnection)
	http.HandleFunc("/ws", wsHandler)
	http.ListenAndServe(addrPort, nil)
}

func (wsConn *wsConnection) HandlePackage(pack *Package) {
	// 未登录拦截
	if pack.Code != CodeSignIn && wsConn.IsSignIn == false {
		wsConn.close()
		return
	}
	switch pack.Code {
	case CodeSignIn:
		wsConn.HandlePackageSignIn(pack)
	case CodeSyncTrigger:
		wsConn.HandlePackageSyncTrigger(pack)
	case CodeMessageSend:
		wsConn.HandlePackageMessageSend(pack)
	case CodeMessageACK:
		wsConn.HandlePackageMessageACK(pack)
	}
	return
}

// HandlePackageSignIn 处理登录消息包
func (wsConn *wsConnection) HandlePackageSignIn(pack *Package) {
	var sign pb.SignIn
	err := proto.Unmarshal(pack.Content, &sign)
	if err != nil {
		logger.Sugar.Error(err)
		wsConn.close()
		return
	}

	transferSignIn := transfer.SignIn{
		DeviceId: sign.DeviceId,
		UserId:   sign.UserId,
		Token:    sign.Token,
	}

	// 处理设备登录逻辑
	ack, err := signIn(transferSignIn)
	if err != nil {
		logger.Sugar.Error(err)
		return
	}

	content, err := proto.Marshal(&pb.SignInACK{Code: int32(ack.Code), Message: ack.Message})
	if err != nil {
		logger.Sugar.Error(err)
		return
	}
	wsPack := Package{Code: CodeSignInACK, Content: content}
	writeBuff, _ := WsEecode(wsPack)
	err = wsConn.wsWrite(websocket.TextMessage, writeBuff)
	if err != nil {
		logger.Sugar.Error(err)
		return
	}

	if ack.Code == transfer.CodeSignInSuccess {
		// 将连接保存到本机字典
		wsConn.IsSignIn = true
		wsConn.DeviceId = sign.DeviceId
		wsConn.UserId = sign.UserId
		myWStore(wsConn.DeviceId, wsConn)

		// 将设备和服务器IP的对应关系保存到redis
		redisClient.Set(deviceIdPre+fmt.Sprint(wsConn.DeviceId), conf.ConnectTCPListenIP+"."+conf.ConnectTCPListenPort,
			0)
	}
}

// HandlePackageSyncTrigger 处理同步触发消息包
func (wsConn *wsConnection) HandlePackageSyncTrigger(pack *Package) {
	var trigger pb.SyncTrigger
	err := proto.Unmarshal(pack.Content, &trigger)
	if err != nil {
		logger.Sugar.Error(err)
		wsConn.close()
		return
	}

	transferTrigger := transfer.SyncTrigger{
		DeviceId:     wsConn.DeviceId,
		UserId:       wsConn.UserId,
		SyncSequence: trigger.SyncSequence,
	}

	publishSyncTrigger(transferTrigger)
}

// HandlePackageMessageSend 处理消息发送包
func (wsConn *wsConnection) HandlePackageMessageSend(pack *Package) {
	var send pb.MessageSend
	err := proto.Unmarshal(pack.Content, &send)
	if err != nil {
		logger.Sugar.Error(err)
		wsConn.close()
		return
	}

	transferSend := transfer.MessageSend{
		SenderDeviceId: wsConn.DeviceId,
		SenderUserId:   wsConn.UserId,
		ReceiverType:   send.ReceiverType,
		ReceiverId:     send.ReceiverId,
		Type:           send.Type,
		Content:        send.Content,
		SendSequence:   send.SendSequence,
		SendTime:       lib.UnunixTime(send.SendTime),
	}

	publishMessageSend(transferSend)
	if err != nil {
		logger.Sugar.Error(err)
	}
}

// HandlePackageMessageACK 处理消息回执消息包
func (wsConn *wsConnection) HandlePackageMessageACK(pack *Package) {
	var ack pb.MessageACK
	err := proto.Unmarshal(pack.Content, &ack)
	if err != nil {
		logger.Sugar.Error(err)
		wsConn.close()
		return
	}

	transferAck := transfer.MessageACK{
		MessageId:    ack.MessageId,
		DeviceId:     wsConn.DeviceId,
		UserId:       wsConn.UserId,
		SyncSequence: ack.SyncSequence,
		ReceiveTime:  lib.UnunixTime(ack.ReceiveTime),
	}

	publishMessageACK(transferAck)
}
