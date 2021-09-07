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

package shared

import (
	"context"
	"database/sql"

	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/pushserver/api"
	"github.com/matrix-org/dendrite/pushserver/storage/tables"
	"github.com/sirupsen/logrus"
)

// See https://matrix.org/docs/spec/client_server/r0.6.1#get-matrix-client-r0-pushers
const pushersSchema = `
-- Stores data about pushers.
CREATE TABLE IF NOT EXISTS pusher_pushers (
	id SERIAL PRIMARY KEY,
	-- The Matrix user ID localpart for this pusher
	localpart TEXT NOT NULL,
	session_id BIGINT DEFAULT NULL,
	profile_tag TEXT,
	kind TEXT NOT NULL,
	app_id TEXT NOT NULL,
	app_display_name TEXT NOT NULL,
	device_display_name TEXT NOT NULL,
	pushkey TEXT NOT NULL,
	lang TEXT NOT NULL,
	format TEXT NOT NULL,
	url TEXT NOT NULL
);

-- For faster deleting by app_id, pushkey pair.
CREATE INDEX IF NOT EXISTS pusher_app_id_pushkey_idx ON pusher_pushers(app_id, pushkey);

-- For faster retrieving by localpart.
CREATE INDEX IF NOT EXISTS pusher_localpart_idx ON pusher_pushers(localpart);

-- Pushkey must be unique for a given user and app.
CREATE UNIQUE INDEX IF NOT EXISTS pusher_app_id_pushkey_localpart_idx ON pusher_pushers(app_id, pushkey, localpart);
`

const insertPusherSQL = "" +
	"INSERT INTO pusher_pushers(localpart, session_id, pushkey, kind, app_id, app_display_name, device_display_name, profile_tag, lang, format, url) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)"

const selectPushersSQL = "" +
	"SELECT session_id, pushkey, kind, app_id, app_display_name, device_display_name, profile_tag, lang, format, url FROM pusher_pushers WHERE localpart = $1"

// const selectPusherSQL = "" +
// 	"SELECT session_id, pushkey, kind, app_id, app_display_name, device_display_name, profile_tag, lang, data FROM pusher_pushers WHERE localpart = $1 AND pushkey = $2 AND app_id = $3"

// const updatePusherSQL = "" +
// 	"UPDATE pusher_pushers SET kind = $1, app_id = $2, app_display_name = $3, device_display_name = $4, profile_tag = $5, lang = $6, data = $7 WHERE localpart = $8 AND pushkey = $9 AND app_id = $3"

const deletePusherSQL = "" +
	"DELETE FROM pusher_pushers WHERE app_id = $1 AND pushkey = $2 AND localpart = $3"

const deletePushersByAppIdAndPushKeySQL = "" +
	"DELETE FROM pusher_pushers WHERE app_id = $1 AND pushkey = $2"

type pushersStatements struct {
	insertPusherStmt  *sql.Stmt
	selectPushersStmt *sql.Stmt
	// selectPusherByPushkeyStmt *sql.Stmt
	// updatePusherStmt             *sql.Stmt
	deletePusherStmt                   *sql.Stmt
	deletePushersByAppIdAndPushKeyStmt *sql.Stmt
}

func CreateMembershipTable(db *sql.DB) error {
	_, err := db.Exec(pushersSchema)
	return err
}

func preparePushersTable(db *sql.DB) (tables.Pusher, error) {
	s := &pushersStatements{}

	return s, sqlutil.StatementList{
		{&s.insertPusherStmt, insertPusherSQL},
		{&s.selectPushersStmt, selectPushersSQL},
		// {&s.selectPusherByPushkeyStmt, selectPusherSQL},
		// {&s.updatePusherStmt, updatePusherSQL},
		{&s.deletePusherStmt, deletePusherSQL},
		{&s.deletePushersByAppIdAndPushKeyStmt, deletePushersByAppIdAndPushKeySQL},
	}.Prepare(db)
}

// insertPusher creates a new pusher.
// Returns an error if the user already has a pusher with the given pusher pushkey.
// Returns nil error success.
func (s *pushersStatements) InsertPusher(
	ctx context.Context, session_id int64,
	pushkey, kind, appid, appdisplayname, devicedisplayname, profiletag, lang, format, url, localpart string,
) error {
	_, err := s.insertPusherStmt.ExecContext(ctx, localpart, session_id, pushkey, kind, appid, appdisplayname, devicedisplayname, profiletag, lang, format, url)
	logrus.Debugf("🥳 Created pusher %d", session_id)
	return err
}

func (s *pushersStatements) SelectPushers(
	ctx context.Context, localpart string,
) ([]api.Pusher, error) {
	pushers := []api.Pusher{}
	rows, err := s.selectPushersStmt.QueryContext(ctx, localpart)

	if err != nil {
		return pushers, err
	}
	defer internal.CloseAndLogIfError(ctx, rows, "SelectPushers: rows.close() failed")

	for rows.Next() {
		var pusher api.Pusher
		err = rows.Scan(
			&pusher.SessionID,
			&pusher.PushKey,
			&pusher.Kind,
			&pusher.AppID,
			&pusher.AppDisplayName,
			&pusher.DeviceDisplayName,
			&pusher.ProfileTag,
			&pusher.Language,
			&pusher.Data.Format,
			&pusher.Data.URL)
		if err != nil {
			return pushers, err
		}
		pushers = append(pushers, pusher)
	}

	logrus.Debugf("🤓 Database returned %d pushers", len(pushers))
	return pushers, rows.Err()
}

// func (s *pushersStatements) SelectPusherByPushkey(
// 	ctx context.Context, localpart, pushkey string,
// ) (*api.Pusher, error) {
// 	var pusher api.Pusher
// 	var sessionid sql.NullInt64
// 	var key, kind, appid, appdisplayname, devicedisplayname, profiletag, lang, data sql.NullString

// 	stmt := s.selectPusherByPushkeyStmt
// 	err := stmt.QueryRowContext(ctx, localpart, pushkey).Scan(&sessionid, &key, &kind, &appid, &appdisplayname, &devicedisplayname, &profiletag, &lang, &data)

// 	if err == nil {
// 		if sessionid.Valid {
// 			pusher.SessionID = sessionid.Int64
// 		}
// 		if key.Valid {
// 			pusher.PushKey = key.String
// 		}
// 		if kind.Valid {
// 			pusher.Kind = kind.String
// 		}
// 		if appid.Valid {
// 			pusher.AppID = appid.String
// 		}
// 		if appdisplayname.Valid {
// 			pusher.AppDisplayName = appdisplayname.String
// 		}
// 		if devicedisplayname.Valid {
// 			pusher.DeviceDisplayName = devicedisplayname.String
// 		}
// 		if profiletag.Valid {
// 			pusher.ProfileTag = profiletag.String
// 		}
// 		if lang.Valid {
// 			pusher.Language = lang.String
// 		}
// 		if data.Valid {
// 			pusher.Data = data.String
// 		}

// 	}

// 	return &pusher, err
// }

// func (s *pushersStatements) UpdatePusher(
// 	ctx context.Context, txn *sql.Tx, pushkey, kind, appid, appdisplayname, devicedisplayname, profiletag, lang, data, localpart string,
// ) error {
// 	stmt := sqlutil.TxStmt(txn, s.updatePusherStmt)
// 	_, err := stmt.ExecContext(ctx, kind, appid, appdisplayname, devicedisplayname, profiletag, lang, data, localpart, pushkey)
// 	return err
// }

// deletePusher removes a single pusher by pushkey and user localpart.
func (s *pushersStatements) DeletePusher(
	ctx context.Context, appid, pushkey, localpart string,
) error {
	_, err := s.deletePusherStmt.ExecContext(ctx, appid, pushkey, localpart)
	return err
}

func (s *pushersStatements) DeletePushers(
	ctx context.Context, appid, pushkey string,
) error {
	_, err := s.deletePushersByAppIdAndPushKeyStmt.ExecContext(ctx, appid, pushkey)
	return err
}
