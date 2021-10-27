package tables

import (
	"context"

	"github.com/matrix-org/dendrite/pushserver/api"
)

type Pusher interface {
	InsertPusher(
		ctx context.Context, session_id int64,
		pushkey string, kind api.PusherKind, appid, appdisplayname, devicedisplayname, profiletag, lang, data, localpart string,
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
	Insert(ctx context.Context, localpart, eventID string, highlight bool, n *api.Notification) error
	DeleteUpTo(ctx context.Context, localpart, roomID, eventID string) error
	UpdateRead(ctx context.Context, localpart, roomID, eventID string, v bool) error
	Select(ctx context.Context, localpart string, fromID int64, limit int, filter NotificationFilter) ([]*api.Notification, int64, error)
	SelectCount(ctx context.Context, localpart string, filter NotificationFilter) (int64, error)
}

type NotificationFilter uint32

const (
	HighlightNotifications NotificationFilter = 1 << iota
	NonHighlightNotifications

	NoNotifications  NotificationFilter = 0
	AllNotifications NotificationFilter = (1 << 32) - 1
)
