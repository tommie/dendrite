// Copyright 2020 The Matrix.org Foundation C.I.C.
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

package deltas

import (
	"database/sql"
	"fmt"

	"github.com/lib/pq"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/util"
	"github.com/sirupsen/logrus"
)

type stateBlockData struct {
	StateSnapshotNID types.StateSnapshotNID
	RoomNID          types.RoomNID
	StateBlockNID    types.StateBlockNID
	EventNIDs        types.EventNIDs
}

func LoadStateBlocksRefactor(m *sqlutil.Migrations) {
	m.AddMigration(UpStateBlocksRefactor, DownStateBlocksRefactor)
}

func UpStateBlocksRefactor(tx *sql.Tx) error {
	logrus.Warn("Performing state block refactor upgrade. Please wait, this may take some time!")
	if _, err := tx.Exec(`ALTER TABLE roomserver_state_block RENAME TO _roomserver_state_block;`); err != nil {
		return fmt.Errorf("tx.Exec: %w", err)
	}
	if _, err := tx.Exec(`ALTER TABLE roomserver_state_snapshots RENAME TO _roomserver_state_snapshots;`); err != nil {
		return fmt.Errorf("tx.Exec: %w", err)
	}
	_, err := tx.Exec(`
		CREATE TABLE IF NOT EXISTS roomserver_state_block (
			-- Local numeric ID for this state data.
			state_block_nid bigserial PRIMARY KEY,
			event_nids bigint[] NOT NULL,
			UNIQUE (event_nids)
		);
	`)
	if err != nil {
		return fmt.Errorf("tx.Exec: %w", err)
	}
	_, err = tx.Exec(`
		CREATE SEQUENCE IF NOT EXISTS roomserver_state_snapshot_nid_seq;
		CREATE TABLE IF NOT EXISTS roomserver_state_snapshots (
			state_snapshot_nid bigint PRIMARY KEY DEFAULT nextval('roomserver_state_snapshot_nid_seq'),
			room_nid bigint NOT NULL,
			state_block_nids bigint[] NOT NULL,
			UNIQUE (room_nid, state_block_nids)
		);
	`)
	if err != nil {
		return fmt.Errorf("tx.Exec: %w", err)
	}
	logrus.Warn("New tables created...")

	var snapshotcount int
	err = tx.QueryRow(`
		SELECT COUNT(DISTINCT state_snapshot_nid) FROM _roomserver_state_snapshots;
	`).Scan(&snapshotcount)
	if err != nil {
		return fmt.Errorf("tx.QueryRow.Scan (count snapshots): %w", err)
	}

	batchsize := 100

	var lastsnapshot types.StateSnapshotNID
	var newblocks types.StateBlockNIDs
	var snapshotrows *sql.Rows

	for batchoffset := 0; batchoffset < snapshotcount; batchoffset += batchsize {
		if batchoffset > snapshotcount {
			batchoffset -= batchoffset - snapshotcount
		}

		snapshotrows, err = tx.Query(`
			SELECT
				state_snapshot_nid,
				room_nid,
				state_block_nid,
				ARRAY_AGG(event_nid) AS event_nids
			FROM (
				SELECT
					_roomserver_state_snapshots.state_snapshot_nid,
					_roomserver_state_snapshots.room_nid,
					_roomserver_state_block.state_block_nid,
					_roomserver_state_block.event_nid
				FROM
					_roomserver_state_snapshots
					JOIN _roomserver_state_block ON _roomserver_state_block.state_block_nid = ANY (_roomserver_state_snapshots.state_block_nids)
				WHERE
					_roomserver_state_snapshots.state_snapshot_nid = ANY ( SELECT DISTINCT
							_roomserver_state_snapshots.state_snapshot_nid
						FROM
							_roomserver_state_snapshots
						LIMIT $1 OFFSET $2)) AS _roomserver_state_block
			GROUP BY
				state_snapshot_nid,
				room_nid,
				state_block_nid;
		`, batchsize, batchoffset)
		if err != nil {
			return fmt.Errorf("tx.Query: %w", err)
		}

		logrus.Warnf("Rewriting snapshots %d-%d of %d...", batchoffset, batchoffset+batchsize, snapshotcount)
		var snapshots []stateBlockData

		for snapshotrows.Next() {
			var snapshot stateBlockData
			var eventsarray pq.Int64Array
			if err = snapshotrows.Scan(&snapshot.StateSnapshotNID, &snapshot.RoomNID, &snapshot.StateBlockNID, &eventsarray); err != nil {
				return fmt.Errorf("rows.Scan: %w", err)
			}
			for _, e := range eventsarray {
				snapshot.EventNIDs = append(snapshot.EventNIDs, types.EventNID(e))
			}
			snapshot.EventNIDs = snapshot.EventNIDs[:util.SortAndUnique(snapshot.EventNIDs)]
			snapshots = append(snapshots, snapshot)
		}

		if err = snapshotrows.Close(); err != nil {
			return fmt.Errorf("snapshots.Close: %w", err)
		}

		for _, snapshot := range snapshots {
			var eventsarray pq.Int64Array
			for _, e := range snapshot.EventNIDs {
				eventsarray = append(eventsarray, int64(e))
			}

			var blocknid types.StateBlockNID
			err = tx.QueryRow(`
				INSERT INTO roomserver_state_block (event_nids)
					VALUES ($1)
					ON CONFLICT (event_nids) DO UPDATE SET event_nids=$1
					RETURNING state_block_nid
			`, eventsarray).Scan(&blocknid)
			if err != nil {
				return fmt.Errorf("tx.QueryRow.Scan (insert new block): %w", err)
			}
			newblocks = append(newblocks, blocknid)

			if snapshot.StateSnapshotNID != lastsnapshot {
				var newblocksarray pq.Int64Array
				for _, b := range newblocks {
					newblocksarray = append(newblocksarray, int64(b))
				}

				var newsnapshot types.StateSnapshotNID
				err = tx.QueryRow(`
				INSERT INTO roomserver_state_snapshots (room_nid, state_block_nids)
					VALUES ($1, $2)
					ON CONFLICT (room_nid, state_block_nids) DO UPDATE SET room_nid=$1
					RETURNING state_snapshot_nid
				`, snapshot.RoomNID, newblocksarray).Scan(&newsnapshot)
				if err != nil {
					return fmt.Errorf("tx.QueryRow.Scan (insert new snapshot): %w", err)
				}

				_, err = tx.Exec(`UPDATE roomserver_events SET state_snapshot_nid=$1 WHERE state_snapshot_nid=$2`, newsnapshot, snapshot.StateSnapshotNID)
				if err != nil {
					return fmt.Errorf("tx.Exec (update events): %w", err)
				}
				_, err = tx.Exec(`UPDATE roomserver_rooms SET state_snapshot_nid=$1 WHERE state_snapshot_nid=$2`, newsnapshot, snapshot.StateSnapshotNID)
				if err != nil {
					return fmt.Errorf("tx.Exec (update rooms): %w", err)
				}

				fmt.Println("Rewrote snapshot", snapshot.StateSnapshotNID, "to", newsnapshot)
				newblocks = newblocks[:0]
				lastsnapshot = snapshot.StateSnapshotNID
			}
		}
	}

	if _, err = tx.Exec(`DROP TABLE _roomserver_state_snapshots;`); err != nil {
		return fmt.Errorf("tx.Exec (delete old snapshot table): %w", err)
	}
	if _, err = tx.Exec(`DROP TABLE _roomserver_state_block;`); err != nil {
		return fmt.Errorf("tx.Exec (delete old block table): %w", err)
	}

	return fmt.Errorf("stopping here to revert changes")
}

func DownStateBlocksRefactor(tx *sql.Tx) error {
	panic("DOWN!")
}
