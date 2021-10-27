package storage

import (
	"context"

	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/pushserver/api"
	"github.com/matrix-org/dendrite/pushserver/storage/tables"
)

type Database interface {
	internal.PartitionStorer
	CreatePusher(ctx context.Context, pusher api.Pusher, localpart string) error
	GetPushers(ctx context.Context, localpart string) ([]api.Pusher, error)
	RemovePusher(ctx context.Context, appId, pushkey, localpart string) error
	RemovePushers(ctx context.Context, appId, pushkey string) error

	InsertNotification(ctx context.Context, localpart, eventID string, tweaks map[string]interface{}, n *api.Notification) error
	SetNotificationRead(ctx context.Context, localpart, roomID, eventID string, b bool) error
	GetNotifications(ctx context.Context, localpart string, fromID int64, limit int, filter NotificationFilter) ([]*api.Notification, int64, error)
}

type NotificationFilter = tables.NotificationFilter
