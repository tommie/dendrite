package storage

import (
	"context"

	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/pushserver/api"
)

type Database interface {
	internal.PartitionStorer
	CreatePusher(ctx context.Context, sessionId int64, pushkey, kind, appid, appdisplayname, devicedisplayname, profiletag, lang, data, localpart string) error
	GetPushersByLocalpart(ctx context.Context, localpart string) ([]api.Pusher, error)
	GetPusherByPushkey(ctx context.Context, pushkey, localpart string) (*api.Pusher, error)
	UpdatePusher(ctx context.Context, pushkey, kind, appid, appdisplayname, devicedisplayname, profiletag, lang, data, localpart string) error
	RemovePusher(ctx context.Context, appId, pushkey, localpart string) error
}
