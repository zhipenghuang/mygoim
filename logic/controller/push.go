package controller

import "goim/logic/service"

func init() {
	g := Engine.Group("/push")
	g.POST("/toSingle", handler(PushController{}.ToSingle))
	g.POST("/toGroup", handler(PushController{}.ToGroup))
}

type PushController struct{}

// Tosingle 推送单个用户
func (PushController) ToSingle(c *context) {
	var data struct {
		ToUserIds []int64 `json:"toUserIds"`
		Content   string  `json:"content"`
		Operate   string  `json:"operate"`
	}
	if c.bindJson(&data) != nil {
		return
	}
	c.response(service.PushService.ToSingle(Context(), data.ToUserIds, data.Content, data.Operate))
}

// Tosingle 推送单个用户
func (PushController) ToGroup(c *context) {
	var data struct {
		ToGroupIds []int64 `json:"toGroupIds"`
		Content    string  `json:"content"`
		Operate    string  `json:"operate"`
	}
	if c.bindJson(&data) != nil {
		return
	}
	c.response(service.PushService.ToGroup(Context(), data.ToGroupIds, data.Content, data.Operate))
}
