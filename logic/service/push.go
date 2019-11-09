package service

import (
	"goim/logic/dao"
	"goim/logic/model"
	"goim/logic/mq/produce"
	"goim/public/imctx"
	"goim/public/lib"
	"goim/public/logger"
	"goim/public/transfer"
	"time"
)

type pushService struct{}

var PushService = new(pushService)

func (*pushService) ToSingle(ctx *imctx.Context, toUserIds []int64, content string, operate string) (res interface{}, err error) {
	if len(toUserIds) == 0 {
		users, err := dao.UserDao.All(ctx)
		if err != nil {
			logger.Sugar.Error(err)
			return nil, err
		}
		for _, user := range users {
			toUserIds = append(toUserIds, user.Id)
		}
	}
	for _, toUserId := range toUserIds {
		messageId := lib.Lid.Get()
		userSequence, err := UserRequenceService.GetNext(ctx, toUserId)
		if err != nil {
			logger.Sugar.Error(err)
			return nil, err
		}
		messageItem := transfer.MessageItem{
			MessageId:    messageId,
			SenderType:   SenderTypeOther,
			ReceiverType: 1,
			Content:      content,
			Sequence:     userSequence,
			SendTime:     time.Now(),
		}
		message := model.Message{
			MessageId:      messageId,
			UserId:         toUserId,
			SenderType:     SenderTypeOther,
			SenderId:       0, //先默认系统id
			SenderDeviceId: 0,
			ReceiverType:   1, //发送给个人1，群推送2
			ReceiverId:     toUserId,
			Type:           1, //1文本2语音3视频
			Content:        content,
			Sequence:       userSequence,
			SendTime:       time.Now(),
		}
		// 查询用户在线设备
		devices, err := dao.DeviceDao.ListOnlineByUserId(ctx, toUserId)
		if err != nil {
			logger.Sugar.Error(err)
			return nil, err
		}
		err = MessageService.Add(ctx, message)

		for _, v := range devices {
			transferMessage := transfer.Message{DeviceId: v.Id, Messages: []transfer.MessageItem{messageItem}}
			produce.PublishMessage(transferMessage)

			logger.Sugar.Infow("系统消息推送",
				"device_id:", v.Id,
				"user_id", toUserId,
			)
		}
	}
	return "success", nil
}

func (service *pushService) ToGroup(context *imctx.Context, toGroupIds []int64, content string, operate string) (interface{}, error) {

	for _, groupId := range toGroupIds {
		groupUsers, err := dao.GroupUserDao.ListGroupUser(context, groupId)
		if err != nil {
			logger.Sugar.Error(err)
			return nil, err
		}
		for _, toUser := range groupUsers {
			toUserId := toUser.UserId
			messageId := lib.Lid.Get()
			userSequence, err := UserRequenceService.GetNext(context, toUserId)
			if err != nil {
				logger.Sugar.Error(err)
				return nil, err
			}
			messageItem := transfer.MessageItem{
				MessageId:    messageId,
				SenderType:   SenderTypeOther,
				ReceiverType: 2,
				Content:      content,
				Sequence:     userSequence,
				SendTime:     time.Now(),
			}
			message := model.Message{
				MessageId:      messageId,
				UserId:         toUserId,
				SenderType:     SenderTypeOther,
				SenderId:       0, //先默认系统id
				SenderDeviceId: 0,
				ReceiverType:   2, //发送给个人1，群推送2
				ReceiverId:     toUserId,
				Type:           1, //1文本2语音3视频
				Content:        content,
				Sequence:       userSequence,
				SendTime:       time.Now(),
			}
			// 查询用户在线设备
			devices, err := dao.DeviceDao.ListOnlineByUserId(context, toUserId)
			if err != nil {
				logger.Sugar.Error(err)
				return nil, err
			}
			err = MessageService.Add(context, message)

			for _, v := range devices {
				transferMessage := transfer.Message{DeviceId: v.Id, Messages: []transfer.MessageItem{messageItem}}
				produce.PublishMessage(transferMessage)

				logger.Sugar.Infow("系统消息推送",
					"device_id:", v.Id,
					"user_id", toUserId,
				)
			}
		}
	}
	return "success", nil
}
