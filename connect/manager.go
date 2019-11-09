package connect

import "sync"

var manager sync.Map

// store 存储
func store(deviceId int64, ctx *ConnContext) {
	manager.Store(deviceId, ctx)
}
// store 存储
func myWStore(deviceId int64, ctx *wsConnection) {
	manager.Store(deviceId, ctx)
}
// load 获取
func load(deviceId int64) *ConnContext {
	value, ok := manager.Load(deviceId)
	if ok {
		return value.(*ConnContext)
	}
	return nil
}
func myWSload(deviceId int64) *wsConnection {
	value, ok := manager.Load(deviceId)
	if ok {
		return value.(*wsConnection)
	}
	return nil
}

// delete 删除
func myWSDelete(deviceId int64) {
	manager.Delete(deviceId)
}
