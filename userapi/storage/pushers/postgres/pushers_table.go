// Copyright 2021 Dan Peleg <dan@globekeeper.com>
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package postgres

import (
	"context"
	"database/sql"

	"github.com/matrix-org/dendrite/clientapi/userutil"
	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
)

const pushersSchema = `
-- Stores data about pushers.
CREATE TABLE IF NOT EXISTS pusher_pushers (
	-- The Matrix user ID localpart for this pusher
	localpart TEXT NOT NULL PRIMARY KEY,
	-- This is a unique identifier for this pusher. 
	-- The value you should use for this is the routing or destination address information for the notification, for example, 
	-- the APNS token for APNS or the Registration ID for GCM. If your notification client has no such concept, use any unique identifier. 
	-- If the kind is "email", this is the email address to send notifications to.
	-- Max length, 512 bytes.
	pushkey VARCHAR(512) NOT NULL,
	-- The kind of pusher. "http" is a pusher that sends HTTP pokes.
	kind TEXT,
	-- This is a reverse-DNS style identifier for the application. Max length, 64 chars.
	app_id VARCHAR(64),
	-- A string that will allow the user to identify what application owns this pusher.
	app_display_name TEXT,
	-- A string that will allow the user to identify what device owns this pusher.
	device_display_name TEXT,
	-- This string determines which set of device specific rules this pusher executes.
	profile_tag TEXT,
	-- The preferred language for receiving notifications (e.g. 'en' or 'en-US')
	lang TEXT,
	-- Required if kind is http. The URL to use to send notifications to.
	url TEXT,
	-- The format to use when sending notifications to the Push Gateway.
	format TEXT
);

-- Pushkey must be unique for a given user.
CREATE UNIQUE INDEX IF NOT EXISTS pusher_localpart_pushkey_idx ON pusher_pushers(localpart, pushkey);
`

const insertPusherSQL = "" +
	"INSERT INTO pusher_pushers(localpart, pushkey, kind, app_id, app_display_name, device_display_name, profile_tag, lang, url, format) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)"

const selectPushersByLocalpartSQL = "" +
	"SELECT pushkey, kind, app_id, app_display_name, device_display_name, profile_tag, lang, url, format FROM pusher_pushers WHERE localpart = $1"

const selectPusherByPushkeySQL = "" +
	"SELECT pushkey, kind, app_id, app_display_name, device_display_name, profile_tag, lang, url, format FROM pusher_pushers WHERE localpart = $1 AND pushkey = $2"


const deletePusherSQL = "" +
	"DELETE FROM pusher_pushers WHERE pushkey = $1 AND localpart = $2"

type pushersStatements struct {
	insertPusherStmt             *sql.Stmt
	selectPushersByLocalpartStmt *sql.Stmt
	selectPusherByPushkeyStmt    *sql.Stmt
	deletePusherStmt             *sql.Stmt
	serverName                   gomatrixserverlib.ServerName
}

func (s *pushersStatements) execSchema(db *sql.DB) error {
	_, err := db.Exec(pushersSchema)
	return err
}

func (s *pushersStatements) prepare(db *sql.DB, server gomatrixserverlib.ServerName) (err error) {
	if s.insertPusherStmt, err = db.Prepare(insertPusherSQL); err != nil {
		return
	}
	if s.selectPushersByLocalpartStmt, err = db.Prepare(selectPushersByLocalpartSQL); err != nil {
		return
	}
	if s.selectPusherByPushkeyStmt, err = db.Prepare(selectPusherByPushkeySQL); err != nil {
		return
	}
	if s.deletePusherStmt, err = db.Prepare(deletePusherSQL); err != nil {
		return
	}
	s.serverName = server
	return
}

// insertPusher creates a new pusher.
// Returns an error if the user already has a pusher with the given pusher pushkey.
// Returns nil error success.
func (s *pushersStatements) insertPusher(
	ctx context.Context, txn *sql.Tx, pushkey, kind, appid, appdisplayname, devicedisplayname, profiletag, lang, url, format, localpart string,
) error {
	stmt := sqlutil.TxStmt(txn, s.insertPusherStmt)
	_, err := stmt.ExecContext(ctx, localpart, pushkey, kind, appid, appdisplayname, devicedisplayname, profiletag, lang, url, format)
	return err
}

// deletePusher removes a single pusher by pushkey and user localpart.
func (s *pushersStatements) deletePusher(
	ctx context.Context, txn *sql.Tx, pushkey, localpart string,
) error {
	stmt := sqlutil.TxStmt(txn, s.deletePusherStmt)
	_, err := stmt.ExecContext(ctx, pushkey, localpart)
	return err
}

func (s *pushersStatements) selectPushersByLocalpart(
	ctx context.Context, txn *sql.Tx, localpart string,
) ([]api.Pusher, error) {
	pushers := []api.Pusher{}
	rows, err := sqlutil.TxStmt(txn, s.selectPushersByLocalpartStmt).QueryContext(ctx, localpart)

	if err != nil {
		return pushers, err
	}
	defer internal.CloseAndLogIfError(ctx, rows, "selectPushersByLocalpart: rows.close() failed")

	for rows.Next() {
		var pusher api.Pusher
		var pushkey, kind, appid, appdisplayname, devicedisplayname, profiletag, lang, url, format sql.NullString
		err = rows.Scan(&pushkey, &kind, &appid, &appdisplayname, &devicedisplayname, &profiletag, &lang, &url, &format)
		if err != nil {
			return pushers, err
		}
		if pushkey.Valid {
			pusher.PushKey = pushkey.String
		}
		if kind.Valid {
			pusher.Kind = kind.String
		}
		if appid.Valid {
			pusher.AppID = appid.String
		}
		if appdisplayname.Valid {
			pusher.AppDisplayName = appdisplayname.String
		}
		if devicedisplayname.Valid {
			pusher.DeviceDisplayName = devicedisplayname.String
		}
		if profiletag.Valid {
			pusher.ProfileTag = profiletag.String
		}
		if lang.Valid {
			pusher.Language = lang.String
		}
		if url.Valid && format.Valid {
			pusher.Data = api.PusherData{
				URL:    url.String,
				Format: format.String,
			}
		}

		pusher.UserID = userutil.MakeUserID(localpart, s.serverName)
		pushers = append(pushers, pusher)
	}

	return pushers, rows.Err()
}

func (s *pushersStatements) selectPusherByPushkey(
	ctx context.Context, localpart, pushkey string,
) (*api.Pusher, error) {
	var pusher api.Pusher
	var id, key, kind, appid, appdisplayname, devicedisplayname, profiletag, lang, url, format sql.NullString

	stmt := s.selectPusherByPushkeyStmt
	err := stmt.QueryRowContext(ctx, localpart, pushkey).Scan(&id, &key, &kind, &appid, &appdisplayname, &devicedisplayname, &profiletag, &lang, &url, &format)

	if err == nil {
		if key.Valid {
			pusher.PushKey = key.String
		}
		if kind.Valid {
			pusher.Kind = kind.String
		}
		if appid.Valid {
			pusher.AppID = appid.String
		}
		if appdisplayname.Valid {
			pusher.AppDisplayName = appdisplayname.String
		}
		if devicedisplayname.Valid {
			pusher.DeviceDisplayName = devicedisplayname.String
		}
		if profiletag.Valid {
			pusher.ProfileTag = profiletag.String
		}
		if lang.Valid {
			pusher.Language = lang.String
		}
		if url.Valid && format.Valid {
			pusher.Data = api.PusherData{
				URL:    url.String,
				Format: format.String,
			}
		}

		pusher.UserID = userutil.MakeUserID(localpart, s.serverName)
	}

	return &pusher, err
}
