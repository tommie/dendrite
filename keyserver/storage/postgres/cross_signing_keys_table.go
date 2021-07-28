// Copyright 2021 The Matrix.org Foundation C.I.C.
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

	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/keyserver/api"
	"github.com/matrix-org/dendrite/keyserver/storage/tables"
	"github.com/matrix-org/gomatrixserverlib"
)

var crossSigningKeysSchema = `
CREATE TABLE IF NOT EXISTS keyserver_cross_signing_keys (
    user_id TEXT NOT NULL,
	key_type TEXT NOT NULL,
	key_data TEXT NOT NULL,
	stream_id BIGINT NOT NULL 
);

CREATE UNIQUE INDEX IF NOT EXISTS keyserver_cross_signing_keys_idx ON keyserver_cross_signing_keys(user_id, key_type, stream_id);
`

const selectCrossSigningKeysForUserSQL = "" +
	"SELECT DISTINCT ON (user_id, key_type) key_type, key_data FROM keyserver_cross_signing_keys" +
	" WHERE user_id = $1" +
	" ORDER BY user_id, key_type, stream_id DESC"

type crossSigningKeysStatements struct {
	db                                *sql.DB
	selectCrossSigningKeysForUserStmt *sql.Stmt
}

func NewPostgresCrossSigningKeysTable(db *sql.DB) (tables.CrossSigningKeys, error) {
	s := &crossSigningKeysStatements{
		db: db,
	}
	_, err := db.Exec(crossSigningKeysSchema)
	if err != nil {
		return nil, err
	}
	if s.selectCrossSigningKeysForUserStmt, err = db.Prepare(selectCrossSigningKeysForUserSQL); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *crossSigningKeysStatements) SelectCrossSigningKeysForUser(
	ctx context.Context, userID string,
) (r api.CrossSigningKeyMap, err error) {
	rows, err := s.selectCrossSigningKeysForUserStmt.QueryContext(ctx, userID)
	if err != nil {
		return nil, err
	}
	defer internal.CloseAndLogIfError(ctx, rows, "selectCrossSigningKeysForUserStmt: rows.close() failed")
	r = api.CrossSigningKeyMap{}
	for rows.Next() {
		var keyType gomatrixserverlib.CrossSigningKeyPurpose
		var keyData gomatrixserverlib.Base64Bytes
		if err := rows.Scan(&keyType, &keyData); err != nil {
			return nil, err
		}
		r[keyType] = keyData
	}
	return
}
