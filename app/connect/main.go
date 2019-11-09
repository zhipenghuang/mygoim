package main

import (
	"fmt"
	"goim/connect"
	"goim/public/logger"
	"runtime"
)

func main() {
	// 启动nsq消费服务
	go func() {
		defer RecoverPanic()
		connect.StartNsqConsumer()
	}()
	//启动websocket服务器
	connect.StartWebsocket("192.168.1.132:8080")
	// 启动tcp服务器
	//conf := connect.Conf{
	//	Address:      conf.ConnectTCPListenIP + ":" + conf.ConnectTCPListenPort,
	//	MaxConnCount: 100,
	//	AcceptCount:  runtime.NumCPU(),
	//}
	//server := connect.NewTCPServer(conf)
	//server.Start()
}

// RecoverPanic 恢复panic
func RecoverPanic() {
	err := recover()
	if err != nil {
		fmt.Println(logger.Sugar)
		fmt.Println(err)
		logger.Sugar.Error(err)
		logger.Sugar.Error(GetPanicInfo())
	}
}

// PrintStaStack 打印Panic堆栈信息
func GetPanicInfo() string {
	buf := make([]byte, 2048)
	n := runtime.Stack(buf, false)
	return fmt.Sprintf("%s", buf[:n])
}
