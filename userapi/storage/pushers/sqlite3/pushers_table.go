// Copyright 2017 Vector Creations Ltd
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

package sqlite3

import (
	"context"
	"database/sql"

	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/userapi/api"

	"github.com/matrix-org/dendrite/clientapi/userutil"
	"github.com/matrix-org/gomatrixserverlib"
)

const pushersSchema = `
-- This sequence is used for automatic allocation of session_id.
-- CREATE SEQUENCE IF NOT EXISTS pusher_session_id_seq START 1;

-- Stores data about pushers.
CREATE TABLE IF NOT EXISTS pusher_pushers (
		pushkey VARCHAR(512) PRIMARY KEY,
    kind TEXT ,
    app_id VARCHAR(64) ,
    app_display_name TEXT,
    device_display_name TEXT,
    profile_tag TEXT,
    language TEXT,
    url TEXT,
		format TEXT,

		UNIQUE (localpart, pushkey)
);
`
const selectPushersByLocalpartSQL = "" +
	"SELECT pushkey, kind, app_id, app_display_name, device_display_name, profile_tag, language FROM pusher_pushers WHERE localpart = $1"

type pushersStatements struct {
	db                           *sql.DB
	writer                       sqlutil.Writer
	selectPushersByLocalpartStmt *sql.Stmt
	serverName                   gomatrixserverlib.ServerName
}

func (s *pushersStatements) execSchema(db *sql.DB) error {
	_, err := db.Exec(pushersSchema)
	return err
}

func (s *pushersStatements) prepare(db *sql.DB, writer sqlutil.Writer, server gomatrixserverlib.ServerName) (err error) {
	s.db = db
	s.writer = writer
	if s.selectPushersByLocalpartStmt, err = db.Prepare(selectPushersByLocalpartSQL); err != nil {
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

	return pushers, nil
}
