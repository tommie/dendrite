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
	--  This is a unique identifier for this pusher. See /set for more detail. Max length, 512 bytes.
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
	format TEXT,
);

-- Pusher IDs must be unique for a given user.
CREATE UNIQUE INDEX IF NOT EXISTS pusher_localpart_pushkey_idx ON pusher_pushers(localpart, pushkey);
`

const selectPushersByLocalpartSQL = "" +
	"SELECT pushkey, kind, app_id, app_display_name, device_display_name, profile_tag, language FROM pusher_pushers WHERE localpart = $1"

type pushersStatements struct {
	selectPushersByLocalpartStmt *sql.Stmt
	serverName                   gomatrixserverlib.ServerName
}

func (s *pushersStatements) execSchema(db *sql.DB) error {
	_, err := db.Exec(pushersSchema)
	return err
}

func (s *pushersStatements) prepare(db *sql.DB, server gomatrixserverlib.ServerName) (err error) {
	if s.selectPushersByLocalpartStmt, err = db.Prepare(selectPushersByLocalpartSQL); err != nil {
		return
	}
	if s.selectPushersByPushkeyStmt, err = db.Prepare(selectPushersByPushkeySQL); err != nil {
		return
	}
	s.serverName = server
	return
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
		var pushkey, kind, appid, appdisplayname, devicedisplayname, profiletag, language, url, format sql.NullString
		err = rows.Scan(&pushkey, &kind, &appid, &appdisplayname, &devicedisplayname, &profiletag, &language, &url, &format)
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
		if language.Valid {
			pusher.Language = language.String
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
