package shared

import (
	"context"
	"database/sql"

	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/pushserver/api"
	"github.com/matrix-org/dendrite/pushserver/storage/tables"
	"github.com/sirupsen/logrus"
)

type Database struct {
	DB      *sql.DB
	Writer  sqlutil.Writer
	pushers tables.Pusher
}

func (d *Database) CreatePusher(
	ctx context.Context, session_id int64,
	pushkey, kind, appid, appdisplayname, devicedisplayname, profiletag, lang, data, localpart string,
) error {
	return d.pushers.InsertPusher(ctx, nil, session_id, pushkey, kind, appid, appdisplayname, devicedisplayname, profiletag, lang, data, localpart)
}

// GetPushersByLocalpart returns the pushers matching the given localpart.
func (d *Database) GetPushersByLocalpart(
	ctx context.Context, localpart string,
) ([]api.Pusher, error) {
	return d.pushers.SelectPushersByLocalpart(ctx, nil, localpart)
}

// GetPusherByPushkey returns the pusher matching the given localpart.
func (d *Database) GetPusherByPushkey(
	ctx context.Context, pushkey, localpart string,
) (*api.Pusher, error) {
	return d.pushers.SelectPusherByPushkey(ctx, localpart, pushkey)
}

// UpdatePusher updates the given pusher with the display name.
// Returns SQL error if there are problems and nil on success.
func (d *Database) UpdatePusher(
	ctx context.Context, pushkey, kind, appid, appdisplayname, devicedisplayname, profiletag, lang, data, localpart string,
) error {
	return sqlutil.WithTransaction(d.DB, func(txn *sql.Tx) error {
		return d.pushers.UpdatePusher(ctx, txn, pushkey, kind, appid, appdisplayname, devicedisplayname, profiletag, lang, data, localpart)
	})
}

// RemovePusher revokes a pusher by deleting the entry in the database
// matching with the given pushkey and user ID localpart.
// If the pusher doesn't exist, it will not return an error
// If something went wrong during the deletion, it will return the SQL error.
func (d *Database) RemovePusher(
	ctx context.Context, appid, pushkey, localpart string,
) error {
	return sqlutil.WithTransaction(d.DB, func(txn *sql.Tx) error {
		if err := d.pushers.DeletePusher(ctx, txn, appid, pushkey, localpart); err != sql.ErrNoRows {
			return err
		} else {
			logrus.WithError(err).Debug("RemovePusher Error")
		}
		return nil
	})
}

func (d *Database) Prepare() (err error) {
	d.pushers, err = preparePushersTable(d.DB)
	return
}
