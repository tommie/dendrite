package tables

import (
	"context"
	"database/sql"

	"github.com/matrix-org/dendrite/pushserver/api"
)

type Pusher interface {
	InsertPusher(
		ctx context.Context, txn *sql.Tx, session_id int64,
		pushkey, kind, appid, appdisplayname, devicedisplayname, profiletag, lang, data, localpart string,
	) error
	SelectPushersByLocalpart(
		ctx context.Context, txn *sql.Tx, localpart string,
	) ([]api.Pusher, error)
	SelectPusherByPushkey(
		ctx context.Context, localpart, pushkey string,
	) (*api.Pusher, error)
	UpdatePusher(
		ctx context.Context, txn *sql.Tx, pushkey, kind, appid, appdisplayname, devicedisplayname, profiletag, lang, data, localpart string,
	) error
	DeletePusher(
		ctx context.Context, txn *sql.Tx, appid, pushkey, localpart string,
	) error
}

type Notifications interface {
	InsertNotification()
	GetNotifications()
	SetToken()
	GetToken()
}
