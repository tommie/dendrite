package shared

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/matrix-org/dendrite/internal/pushrules"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/pushserver/api"
	"github.com/matrix-org/dendrite/pushserver/storage/tables"
)

type Database struct {
	DB            *sql.DB
	Writer        sqlutil.Writer
	notifications tables.Notifications
	pushers       tables.Pusher
}

func (d *Database) Prepare() (err error) {
	d.notifications, err = prepareNotificationsTable(d.DB)
	if err != nil {
		return
	}
	d.pushers, err = preparePushersTable(d.DB)
	return
}

func (d *Database) InsertNotification(ctx context.Context, localpart, eventID string, tweaks map[string]interface{}, n *api.Notification) error {
	return d.Writer.Do(nil, nil, func(_ *sql.Tx) error {
		return d.notifications.Insert(ctx, localpart, eventID, pushrules.BoolTweakOr(tweaks, pushrules.HighlightTweak, false), n)
	})
}

func (d *Database) SetNotificationRead(ctx context.Context, localpart, roomID, eventID string, b bool) error {
	return d.Writer.Do(nil, nil, func(_ *sql.Tx) error {
		return d.notifications.UpdateRead(ctx, localpart, roomID, eventID, b)
	})
}

func (d *Database) GetNotifications(ctx context.Context, localpart string, fromID int64, limit int, filter tables.NotificationFilter) ([]*api.Notification, int64, error) {
	return d.notifications.Select(ctx, localpart, fromID, limit, filter)
}

func (d *Database) CreatePusher(
	ctx context.Context, p api.Pusher, localpart string,
) error {
	data, err := json.Marshal(p.Data)
	if err != nil {
		return err
	}
	return d.Writer.Do(nil, nil, func(_ *sql.Tx) error {
		return d.pushers.InsertPusher(
			ctx,
			p.SessionID,
			p.PushKey,
			p.Kind,
			p.AppID,
			p.AppDisplayName,
			p.DeviceDisplayName,
			p.ProfileTag,
			p.Language,
			string(data),
			localpart)
	})
}

// GetPushers returns the pushers matching the given localpart.
func (d *Database) GetPushers(
	ctx context.Context, localpart string,
) ([]api.Pusher, error) {
	return d.pushers.SelectPushers(ctx, localpart)
}

// RemovePusher deletes one pusher
// Invoked when `append` is true and `kind` is null in
// https://matrix.org/docs/spec/client_server/r0.6.1#post-matrix-client-r0-pushers-set
func (d *Database) RemovePusher(
	ctx context.Context, appid, pushkey, localpart string,
) error {
	return d.Writer.Do(nil, nil, func(_ *sql.Tx) error {
		err := d.pushers.DeletePusher(ctx, appid, pushkey, localpart)
		if err == sql.ErrNoRows {
			return nil
		}
		return err
	})
}

// RemovePushers deletes all pushers that match given App Id and Push Key pair.
// Invoked when `append` parameter is false in
// https://matrix.org/docs/spec/client_server/r0.6.1#post-matrix-client-r0-pushers-set
func (d *Database) RemovePushers(
	ctx context.Context, appid, pushkey string,
) error {
	return d.Writer.Do(nil, nil, func(_ *sql.Tx) error {
		return d.pushers.DeletePushers(ctx, appid, pushkey)
	})
}
