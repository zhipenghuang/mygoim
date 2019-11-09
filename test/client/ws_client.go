package client

import (
	"fmt"
	"github.com/golang/protobuf/proto"
	"github.com/gorilla/websocket"
	"goim/connect"
	"goim/public/lib"
	"goim/public/pb"
	"goim/public/transfer"
	"log"
	"net/url"
	"time"
)

type WsClient struct {
	DeviceId     int64
	UserId       int64
	Token        string
	SendSequence int64
	SyncSequence int64
	Codec        *connect.WsCodec
}

func (c *WsClient) Start() {

	u := url.URL{Scheme: "ws", Host: "localhost:8080", Path: "/echo"}
	log.Printf("connecting to %s", u.String())

	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		log.Fatal("dial:", err)
	}
	defer conn.Close()

	c.Codec = connect.NewWsCodec(conn)
	c.SignIn()
	c.SyncTrigger()
	go c.HeadBeat()
	go c.Receive()
	select {}
}

func (c *WsClient) SignIn() {
	signIn := pb.SignIn{
		DeviceId: c.DeviceId,
		UserId:   c.UserId,
		Token:    c.Token,
	}

	signInBytes, err := proto.Marshal(&signIn)
	if err != nil {
		fmt.Println(err)
		return
	}

	wpack := connect.Package{Code: connect.CodeSignIn, Content: signInBytes}
	writeBuff, _ := connect.Eecode11(wpack)
	err1 := c.Codec.Conn.WriteMessage(websocket.TextMessage, writeBuff)
	if err1 != nil {
		fmt.Println(err1)
		return
	}
}

func (c *WsClient) SyncTrigger() {
	bytes, err := proto.Marshal(&pb.SyncTrigger{SyncSequence: c.SyncSequence})
	if err != nil {
		fmt.Println(err)
		return
	}
	wpack := connect.Package{Code: connect.CodeSyncTrigger, Content: bytes}
	writeBuff, _ := connect.Eecode11(wpack)
	err1 := c.Codec.Conn.WriteMessage(websocket.TextMessage, writeBuff)

	if err1 != nil {
		fmt.Println(err1)
		return
	}
}

func (c *WsClient) HeadBeat() {
	ticker := time.NewTicker(time.Minute * 4)
	for _ = range ticker.C {
		wpack := connect.Package{Code: connect.CodeHeadbeat, Content: []byte{}}
		writeBuff, _ := connect.Eecode11(wpack)
		err1 := c.Codec.Conn.WriteMessage(websocket.TextMessage, writeBuff)
		if err1 != nil {
			fmt.Println(err1)
			return
		}
	}
}

func (c *WsClient) Receive() {

	for {
		_, message, _ := c.Codec.Conn.ReadMessage()
		//protobuf的messageID
		protoMsgID := int(message[1])
		//去掉头
		protoMsgContent := message[4:]
		pack := &connect.Package{Code: protoMsgID, Content: protoMsgContent}
		c.HandlePackage(*pack)
	}

}

func (c *WsClient) HandlePackage(pack connect.Package) error {
	switch pack.Code {
	case connect.CodeSignInACK:
		ack := pb.SignInACK{}
		err := proto.Unmarshal(pack.Content, &ack)
		if err != nil {
			fmt.Println(err)
			return err
		}
		if ack.Code == 1 {
			fmt.Println("设备登录成功")
			return nil
		}
		fmt.Println("设备登录失败")

	case connect.CodeHeadbeatACK:
	case connect.CodeMessageSendACK:
		ack := pb.MessageSendACK{}
		err := proto.Unmarshal(pack.Content, &ack)
		if err != nil {
			fmt.Println(err)
			return err
		}
		fmt.Println(ack.SendSequence, ack.Code)
	case connect.CodeMessage:
		message := pb.Message{}
		err := proto.Unmarshal(pack.Content, &message)
		if err != nil {
			fmt.Println(err)
			return err
		}

		if message.Type == transfer.MessageTypeSync {
			fmt.Println("消息同步开始......")
		}

		for _, v := range message.Messages {
			if v.ReceiverType == 1 {
				if v.SenderDeviceId != c.DeviceId {
					fmt.Printf("单聊：来自用户：%d,消息内容：%s\n", v.SenderId, v.Content)
				}
			}
			if v.ReceiverType == 2 {
				if v.SenderDeviceId != c.DeviceId {
					fmt.Printf("群聊：来自用户：%d,群组：%d,消息内容：%s\n", v.SenderId, v.ReceiverId, v.Content)
				}
			}
		}

		if message.Type == transfer.MessageTypeSync {
			fmt.Println("消息同步结束")
		}

		if len(message.Messages) == 0 {
			return nil
		}

		ack := pb.MessageACK{
			MessageId:    message.Messages[len(message.Messages)-1].MessageId,
			SyncSequence: message.Messages[len(message.Messages)-1].SyncSequence,
			ReceiveTime:  lib.UnixTime(time.Now()),
		}
		ackBytes, err := proto.Marshal(&ack)
		if err != nil {
			fmt.Println(err)
			return err
		}

		c.SyncSequence = ack.SyncSequence
		wpack := connect.Package{Code: connect.CodeMessageACK, Content: ackBytes}
		writeBuff, _ := connect.Eecode11(wpack)
		err1 := c.Codec.Conn.WriteMessage(websocket.TextMessage, writeBuff)
		if err1 != nil {
			fmt.Println(err1)
			return err1
		}
	case connect.CodePushMessage:
		message := pb.PushMessage{}
		err := proto.Unmarshal(pack.Content, &message)
		if err != nil {
			fmt.Println(err)
			return err
		}
		for _, v := range message.Messages {
			fmt.Printf("系统消息：消息内容：%s\n", v.Content)
		}
		if len(message.Messages) == 0 {
			return nil
		}
	default:
		fmt.Println("switch other")
	}
	return nil
}

func (c *WsClient) SendMessage() {
	send := pb.MessageSend{}
	fmt.Scanf("%d %d %s", &send.ReceiverType, &send.ReceiverId, &send.Content)
	send.Type = 1
	c.SendSequence++
	send.SendSequence = c.SendSequence
	send.SendTime = lib.UnixTime(time.Now())
	bytes, err := proto.Marshal(&send)
	if err != nil {
		fmt.Println(err)
		return
	}
	wpack := connect.Package{Code: connect.CodeMessageSend, Content: bytes}
	writeBuff, _ := connect.Eecode11(wpack)
	err1 := c.Codec.Conn.WriteMessage(websocket.TextMessage, writeBuff)
	if err1 != nil {
		fmt.Println(err1)
	}
}
