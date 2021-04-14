// Copyright 2017-2018 New Vector Ltd
// Copyright 2019-2020 The Matrix.org Foundation C.I.C.
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
	"fmt"

	"github.com/lib/pq"
	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/roomserver/storage/shared"
	"github.com/matrix-org/dendrite/roomserver/storage/tables"
	"github.com/matrix-org/dendrite/roomserver/types"
)

const stateSchema = `
CREATE TABLE IF NOT EXISTS roomserver_state (
    state_nid BIGSERIAL PRIMARY KEY,
	room_nid bigint NOT NULL,
    event_nids bigint[] NOT NULL,
    UNIQUE (room_nid, event_nids),
	CONSTRAINT fk_room_id FOREIGN KEY(room_nid) REFERENCES roomserver_rooms(room_nid)
);
`

const insertNewStateSnapshotSQL = "" +
	"INSERT INTO roomserver_state (room_nid, event_nids)" +
	" VALUES ($1, $2)" +
	" ON CONFLICT (room_nid, event_nids) DO UPDATE SET room_nid = $1" +
	" RETURNING state_nid"

const bulkSelectNewStateSnapshotSQL = "" +
	"SELECT state_nid, event_nids" +
	" FROM roomserver_state WHERE state_nid = ANY($1)" +
	" ORDER BY state_nid"

type stateStatements struct {
	insertStateStmt     *sql.Stmt
	bulkSelectStateStmt *sql.Stmt
}

func NewPostgresStateTable(db *sql.DB) (tables.State, error) {
	s := &stateStatements{}
	_, err := db.Exec(stateSchema)
	if err != nil {
		return nil, err
	}

	return s, shared.StatementList{
		{&s.insertStateStmt, insertNewStateSnapshotSQL},
		{&s.bulkSelectStateStmt, bulkSelectNewStateSnapshotSQL},
	}.Prepare(db)
}

func (s *stateStatements) InsertState(
	ctx context.Context,
	txn *sql.Tx,
	roomNID types.RoomNID,
	eventNIDs []types.EventNID,
) (types.StateSnapshotNID, error) {
	stmt := sqlutil.TxStmt(txn, s.insertStateStmt)
	var id int64
	var err error
	eventNIDs = types.DeduplicateEventNIDs(eventNIDs)
	if err = stmt.QueryRowContext(ctx, int64(roomNID), eventNIDsAsArray(eventNIDs)).Scan(&id); err != nil {
		return 0, fmt.Errorf("stmt.ExecContext: %w", err)
	}
	return types.StateSnapshotNID(id), err
}

func (s *stateStatements) BulkSelectState(
	ctx context.Context, stateNIDs []types.StateSnapshotNID,
) (map[types.StateSnapshotNID][]types.EventNID, error) {
	rows, err := s.bulkSelectStateStmt.QueryContext(ctx, stateSnapshotNIDsAsArray(stateNIDs))
	if err != nil {
		return nil, err
	}
	defer internal.CloseAndLogIfError(ctx, rows, "bulkSelectStateBlockEntries: rows.close() failed")

	results := map[types.StateSnapshotNID][]types.EventNID{}
	for rows.Next() {
		var stateNID int64
		var eventNIDs pq.Int64Array
		if err = rows.Scan(&stateNID, &eventNIDs); err != nil {
			return nil, err
		}
		for _, id := range eventNIDs {
			results[types.StateSnapshotNID(stateNID)] = append(
				results[types.StateSnapshotNID(stateNID)],
				types.EventNID(id),
			)
		}
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("rows.Err: %w", err)
	}
	if len(results) != len(stateNIDs) {
		return nil, fmt.Errorf("storage: state data NIDs missing from the database (%d != %d)", len(results), len(stateNIDs))
	}
	return results, err
}

func stateSnapshotNIDsAsArray(stateSnapshotNIDs []types.StateSnapshotNID) pq.Int64Array {
	nids := make([]int64, len(stateSnapshotNIDs))
	for i := range stateSnapshotNIDs {
		nids[i] = int64(stateSnapshotNIDs[i])
	}
	return nids
}
