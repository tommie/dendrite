package shared

import (
	"context"
	"database/sql"

	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/pushserver/api"
	"github.com/matrix-org/dendrite/pushserver/storage/tables"
)

type Database struct {
	DB      *sql.DB
	Writer  sqlutil.Writer
	pushers tables.Pusher
}

func (d *Database) CreatePusher(
	ctx context.Context, p api.Pusher, localpart string,
) error {
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
			p.Data.Format,
			p.Data.URL,
			localpart)
	})
}

// GetPushers returns the pushers matching the given localpart.
func (d *Database) GetPushers(
	ctx context.Context, localpart string,
) ([]api.Pusher, error) {
	return d.pushers.SelectPushers(ctx, localpart)
}

// // GetPusherByPushkey returns the pusher matching the given localpart.
// func (d *Database) GetPusherByPushkey(
// 	ctx context.Context, pushkey, localpart string,
// ) (*api.Pusher, error) {
// 	return d.pushers.SelectPusherByPushkey(ctx, localpart, pushkey)
// }

// // UpdatePusher updates the given pusher with the display name.
// // Returns SQL error if there are problems and nil on success.
// func (d *Database) UpdatePusher(
// 	ctx context.Context, pushkey, kind, appid, appdisplayname, devicedisplayname, profiletag, lang, data, localpart string,
// ) error {
// 	return sqlutil.WithTransaction(d.DB, func(txn *sql.Tx) error {
// 		return d.pushers.UpdatePusher(ctx, txn, pushkey, kind, appid, appdisplayname, devicedisplayname, profiletag, lang, data, localpart)
// 	})
// }

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

func (d *Database) Prepare() (err error) {
	d.pushers, err = preparePushersTable(d.DB)
	return
}
