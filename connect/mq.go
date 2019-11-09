package connect

import (
	"time"

	"encoding/json"
	"goim/public/lib"
	"goim/public/logger"
	"goim/public/pb"
	"goim/public/transfer"

	"goim/conf"

	"github.com/golang/protobuf/proto"
	"github.com/gorilla/websocket"
	"github.com/json-iterator/go"
	"github.com/nsqio/go-nsq"
)

var producer *nsq.Producer

// NsqConsumer 消费消息
func NsqConsumer(topic, channel string, handle func(message *nsq.Message) error, concurrency int) {
	config := nsq.NewConfig()
	config.LookupdPollInterval = 1 * time.Second

	consumer, err := nsq.NewConsumer(topic, channel, config)
	if err != nil {
		logger.Sugar.Error(err)
		panic(err)
	}
	consumer.AddConcurrentHandlers(nsq.HandlerFunc(handle), concurrency)
	err = consumer.ConnectToNSQD(conf.NSQIP)
	if err != nil {
		panic(err)
	}
}

// StartNsqConsume 启动nsq消费者，以后所有的消费者在这里注册
func StartNsqConsumer() {
	NsqConsumer(conf.ConnectTCPListenIP+"."+conf.ConnectTCPListenPort+".message", "1", handleMessage, 20)
	NsqConsumer(conf.ConnectTCPListenIP+"."+conf.ConnectTCPListenPort+".message_send_ack", "1", handleMessageSendACK, 20)
}

// handleMessage 处理消息投递
func handleMessage(msg *nsq.Message) error {
	// nsq消息解码
	var message transfer.Message
	err := json.Unmarshal(msg.Body, &message)
	if err != nil {
		logger.Sugar.Error(err)
		return nil
	}

	// 获取设备对应的TCP连接
	ctx := myWSload(message.DeviceId)
	if ctx == nil {
		logger.Sugar.Error("ctx id nil")
		return nil
	}

	messages := make([]*pb.MessageItem, 0, len(message.Messages))
	for _, v := range message.Messages {
		item := new(pb.MessageItem)

		item.MessageId = v.MessageId
		item.SenderType = int32(v.SenderType)
		item.SenderId = v.SenderId
		item.SenderDeviceId = v.SenderDeviceId
		item.ReceiverType = int32(v.ReceiverType)
		item.ReceiverId = v.ReceiverId
		item.Type = int32(v.Type)
		item.Content = v.Content
		item.SyncSequence = v.Sequence
		item.SendTime = lib.UnixTime(v.SendTime)

		messages = append(messages, item)
	}

	// 消息编码
	content, err := proto.Marshal(&pb.Message{Type: message.Type, Messages: messages})
	if err != nil {
		logger.Sugar.Error(err)
		return err
	}

	// 发送消息
	//err = ctx.Codec.Eecode(Package{Code: CodeMessage, Content: content}, WriteDeadline)
	//if err != nil {
	//	logger.Sugar.Error(err)
	//	return err
	//}
	wpack := Package{Code: CodeMessage, Content: content}
	writeBuff, _ := WsEecode(wpack)
	//err11 := ctx.Codec.Conn.WriteMessage(websocket.TextMessage, writeBuff)
	err = ctx.wsWrite(websocket.TextMessage, writeBuff)
	if err != nil {
		logger.Sugar.Error(err)
	}
	return nil
}

// handleMessageSendACK 处理消息发送回执
func handleMessageSendACK(msg *nsq.Message) error {
	// nsq消息解码
	var ack transfer.MessageSendACK
	err := json.Unmarshal(msg.Body, &ack)
	if err != nil {
		logger.Sugar.Error(err)
		return nil
	}

	// 消息编码
	content, err := proto.Marshal(&pb.MessageSendACK{SendSequence: ack.SendSequence, Code: int32(ack.Code)})
	if err != nil {
		logger.Sugar.Error(err)
		return err
	}

	// 获取设备对应的TCP连接
	ctx := myWSload(ack.DeviceId)
	if ctx == nil {
		logger.Sugar.Error(err)
		return err
	}

	// 发送消息
	//err = ctx.Codec.Eecode(Package{Code: CodeMessageSendACK, Content: content}, WriteDeadline)
	//if err != nil {
	//	logger.Sugar.Error(err)
	//	return err
	//}
	wsPack := Package{Code: CodeMessageSendACK, Content: content}
	writeBuff, _ := WsEecode(wsPack)
	err = ctx.wsWrite(websocket.TextMessage, writeBuff)
	if err != nil {
		logger.Sugar.Error(err)
	}
	return nil
}

func init() {
	var err error
	cfg := nsq.NewConfig()
	producer, err = nsq.NewProducer(conf.NSQIP, cfg)
	if nil != err {
		panic("nsq new panic")
	}

	err = producer.Ping()
	if nil != err {
		panic("nsq ping panic")
	}
}

// publishSyncTrigger 发布消息同步
func publishSyncTrigger(syncTrigger transfer.SyncTrigger) {
	body, err := jsoniter.Marshal(syncTrigger)
	if err != nil {
		logger.Sugar.Error(err)
		return
	}
	err = producer.Publish("sync_trigger", body)
	if err != nil {
		logger.Sugar.Error(err)
	}
}

// publishMessageSend 发布消息发送
func publishMessageSend(send transfer.MessageSend) {
	body, err := jsoniter.Marshal(send)
	if err != nil {
		logger.Sugar.Error(err)
		return
	}
	err = producer.Publish("message_send", body)
	if err != nil {
		logger.Sugar.Error(err)
	}
}

// publishMessageACK 发布消息回执
func publishMessageACK(ack transfer.MessageACK) {
	body, err := jsoniter.Marshal(ack)
	if err != nil {
		logger.Sugar.Error(err)
		return
	}
	err = producer.Publish("message_ack", body)
	if err != nil {
		logger.Sugar.Error(err)
	}
}

// publishOffLine 发布消息回执
func publishOffLine(offLine transfer.OffLine) {
	body, err := jsoniter.Marshal(offLine)
	if err != nil {
		logger.Sugar.Error(err)
		return
	}
	err = producer.Publish("off_line", body)
	if err != nil {
		logger.Sugar.Error(err)
	}
}
