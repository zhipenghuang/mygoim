package main

import (
	"fmt"
	"github.com/gorilla/websocket"
	"goim/connect"
	"goim/test/client"
	"log"
	"net/url"
	"os"
	"os/signal"
)

func main() {
	TestWebScoket()
}

func TestClient() {
	client := client.TcpClient{}
	fmt.Println("input UserId,DeviceId,Token,SendSequence,SyncSequence")
	fmt.Scanf("%d %d %s %d %d", &client.UserId, &client.DeviceId, &client.Token, &client.SendSequence, &client.SyncSequence)
	client.Start()
	for {
		client.SendMessage()
	}
}

func TestDebugClient() {
	client := client.TcpClient{}
	fmt.Println("input UserId,DeviceId,Token,SendSequence,SyncSequence")
	fmt.Scanf("%d %d %s %d %d", &client.UserId, &client.DeviceId, &client.Token, &client.SendSequence, &client.SyncSequence)
	client.Start()
	for {
		client.SendMessage()
	}
}

func TestWebScoketClient() {
	client := client.WsClient{}
	fmt.Println("input UserId,DeviceId,Token,SendSequence,SyncSequence")
	fmt.Scanf("%d %d %s %d %d", &client.UserId, &client.DeviceId, &client.Token, &client.SendSequence, &client.SyncSequence)
	client.Start()
	for {
		client.SendMessage()
	}
}
func TestWebScoket() {

	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)

	u := url.URL{Scheme: "ws", Host: "192.168.1.132:8080", Path: "/ws"}
	log.Printf("connecting to %s", u.String())

	c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		log.Fatal("dial:", err)
	}
	defer c.Close()

	done := make(chan struct{})

	client := client.WsClient{}
	fmt.Println("input UserId,DeviceId,Token,SendSequence,SyncSequence")
	fmt.Scanf("%d %d %s %d %d", &client.UserId, &client.DeviceId, &client.Token, &client.SendSequence, &client.SyncSequence)
	client.Codec = connect.NewWsCodec(c)
	client.SignIn()
	go func() {
		defer close(done)
		for {
			_, message, err := c.ReadMessage()
			if err != nil {
				log.Println("read:", err)
				return
			}
			protoMsgID := int(message[1])
			//去掉头
			protoMsgContent := message[4:]
			pack := connect.Package{Code: protoMsgID, Content: protoMsgContent}
			client.HandlePackage(pack)
		}
	}()

	for {
		select {
		case <-done:
			return
		case <-interrupt:
			log.Println("interrupt")
			return
		default:
			client.SendMessage()
		}
	}
}
