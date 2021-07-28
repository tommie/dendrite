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

package sqlite3

import (
	"database/sql"

	"github.com/matrix-org/dendrite/keyserver/storage/tables"
)

var crossSigningSigsSchema = `
CREATE TABLE IF NOT EXISTS keyserver_cross_signing_sigs (
    user_id TEXT NOT NULL,
	key_id TEXT NOT NULL,
	target_user_id TEXT NOT NULL,
	target_device_id TEXT NOT NULL,
	signature TEXT NOT NULL 
);

CREATE UNIQUE INDEX IF NOT EXISTS keyserver_cross_signing_sigs_idx ON keyserver_cross_signing_sigs(user_id, target_user_id, target_device_id);
`

type crossSigningSigsStatements struct {
	db *sql.DB
}

func NewSqliteCrossSigningSigsTable(db *sql.DB) (tables.CrossSigningSigs, error) {
	s := &crossSigningSigsStatements{
		db: db,
	}
	_, err := db.Exec(crossSigningSigsSchema)
	if err != nil {
		return nil, err
	}
	return s, nil
}
