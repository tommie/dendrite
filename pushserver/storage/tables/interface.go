package tables

import (
	"context"

	"github.com/matrix-org/dendrite/pushserver/api"
)

type Pusher interface {
	InsertPusher(
		ctx context.Context, session_id int64,
		pushkey, kind, appid, appdisplayname, devicedisplayname, profiletag, lang, data, localpart string,
	) error
	SelectPushers(
		ctx context.Context, localpart string,
	) ([]api.Pusher, error)
	DeletePusher(
		ctx context.Context, appid, pushkey, localpart string,
	) error
	DeletePushers(
		ctx context.Context, appid, pushkey string,
	) error
}

type Notifications interface {
	InsertNotification()
	GetNotifications()
	SetToken()
	GetToken()
}
