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
	"encoding/json"

	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/pushserver/api"
	"github.com/matrix-org/dendrite/pushserver/storage/tables"
	"github.com/sirupsen/logrus"
)

// See https://matrix.org/docs/spec/client_server/r0.6.1#get-matrix-client-r0-pushers
const pushersSchema = `
CREATE TABLE IF NOT EXISTS pushserver_pushers (
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
	data TEXT NOT NULL
);

-- For faster deleting by app_id, pushkey pair.
CREATE INDEX IF NOT EXISTS pusher_app_id_pushkey_idx ON pushserver_pushers(app_id, pushkey);

-- For faster retrieving by localpart.
CREATE INDEX IF NOT EXISTS pusher_localpart_idx ON pushserver_pushers(localpart);

-- Pushkey must be unique for a given user and app.
CREATE UNIQUE INDEX IF NOT EXISTS pusher_app_id_pushkey_localpart_idx ON pushserver_pushers(app_id, pushkey, localpart);
`

const insertPusherSQL = "" +
	"INSERT INTO pushserver_pushers (localpart, session_id, pushkey, kind, app_id, app_display_name, device_display_name, profile_tag, lang, data) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)"

const selectPushersSQL = "" +
	"SELECT session_id, pushkey, kind, app_id, app_display_name, device_display_name, profile_tag, lang, data FROM pushserver_pushers WHERE localpart = $1"

const deletePusherSQL = "" +
	"DELETE FROM pushserver_pushers WHERE app_id = $1 AND pushkey = $2 AND localpart = $3"

const deletePushersByAppIdAndPushKeySQL = "" +
	"DELETE FROM pushserver_pushers WHERE app_id = $1 AND pushkey = $2"

type pushersStatements struct {
	insertPusherStmt                   *sql.Stmt
	selectPushersStmt                  *sql.Stmt
	deletePusherStmt                   *sql.Stmt
	deletePushersByAppIdAndPushKeyStmt *sql.Stmt
}

func CreatePushersTable(db *sql.DB) error {
	_, err := db.Exec(pushersSchema)
	return err
}

func preparePushersTable(db *sql.DB) (tables.Pusher, error) {
	s := &pushersStatements{}

	return s, sqlutil.StatementList{
		{&s.insertPusherStmt, insertPusherSQL},
		{&s.selectPushersStmt, selectPushersSQL},
		{&s.deletePusherStmt, deletePusherSQL},
		{&s.deletePushersByAppIdAndPushKeyStmt, deletePushersByAppIdAndPushKeySQL},
	}.Prepare(db)
}

// insertPusher creates a new pusher.
// Returns an error if the user already has a pusher with the given pusher pushkey.
// Returns nil error success.
func (s *pushersStatements) InsertPusher(
	ctx context.Context, session_id int64,
	pushkey string, kind api.PusherKind, appid, appdisplayname, devicedisplayname, profiletag, lang, data, localpart string,
) error {
	_, err := s.insertPusherStmt.ExecContext(ctx, localpart, session_id, pushkey, kind, appid, appdisplayname, devicedisplayname, profiletag, lang, data)
	logrus.Debugf("Created pusher %d", session_id)
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
		var data []byte
		err = rows.Scan(
			&pusher.SessionID,
			&pusher.PushKey,
			&pusher.Kind,
			&pusher.AppID,
			&pusher.AppDisplayName,
			&pusher.DeviceDisplayName,
			&pusher.ProfileTag,
			&pusher.Language,
			&data)
		if err != nil {
			return pushers, err
		}
		err := json.Unmarshal(data, &pusher.Data)
		if err != nil {
			return pushers, err
		}
		pushers = append(pushers, pusher)
	}

	logrus.Debugf("Database returned %d pushers", len(pushers))
	return pushers, rows.Err()
}

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
