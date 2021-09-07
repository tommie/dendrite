package tables

import (
	"context"

	"github.com/matrix-org/dendrite/pushserver/api"
)

type Pusher interface {
	InsertPusher(
		ctx context.Context, session_id int64,
		pushkey, kind, appid, appdisplayname, devicedisplayname, profiletag, lang, format, url, localpart string,
	) error
	SelectPushers(
		ctx context.Context, localpart string,
	) ([]api.Pusher, error)
	// SelectPusherByPushkey(
	// 	ctx context.Context, localpart, pushkey string,
	// ) (*api.Pusher, error)
	// UpdatePusher(
	// 	ctx context.Context, txn *sql.Tx, pushkey, kind, appid, appdisplayname, devicedisplayname, profiletag, lang, data, localpart string,
	// ) error
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
