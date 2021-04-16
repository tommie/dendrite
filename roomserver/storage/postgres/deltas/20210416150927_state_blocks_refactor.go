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
	"context"
	"database/sql"
	"fmt"

	"github.com/lib/pq"
	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/util"
	"github.com/sirupsen/logrus"
)

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
	snapshotrows, err := tx.Query(`SELECT state_snapshot_nid, room_nid, state_block_nids FROM _roomserver_state_snapshots;`)
	if err != nil {
		return fmt.Errorf("tx.Query: %w", err)
	}
	defer internal.CloseAndLogIfError(context.TODO(), snapshotrows, "rows.close() failed")
	for snapshotrows.Next() {
		var snapshot types.StateSnapshotNID
		var room types.RoomNID
		var blocksarray pq.Int64Array
		var blocks []types.StateBlockNID
		if err = snapshotrows.Scan(&snapshot, &room, &blocksarray); err != nil {
			return fmt.Errorf("rows.Scan: %w", err)
		}
		for _, b := range blocksarray {
			blocks = append(blocks, types.StateBlockNID(b))
		}

		var newblocks []types.StateBlockNID
		for _, block := range blocks {
			if err = func() error {
				blockrows, berr := tx.Query(`SELECT event_nid FROM _roomserver_state_block WHERE state_block_nid = $1`, int64(block))
				if berr != nil {
					return fmt.Errorf("tx.Query (event nids from old block): %w", berr)
				}
				defer internal.CloseAndLogIfError(context.TODO(), blockrows, "rows.close() failed")
				events := types.EventNIDs{}
				for blockrows.Next() {
					var event types.EventNID
					if err = blockrows.Scan(&event); err != nil {
						return fmt.Errorf("rows.Scan (event nids from old block): %w", err)
					}
					events = append(events, event)
				}
				events = events[:util.SortAndUnique(events)]

				var blocknid types.StateBlockNID
				err = tx.QueryRow(`
					INSERT INTO roomserver_state_block (event_nids)
						VALUES ($1)
						ON CONFLICT (event_nids) DO UPDATE SET event_nids=$1
						RETURNING state_block_nid
				`, events).Scan(&blocknid)
				if err != nil {
					return fmt.Errorf("tx.QueryRow.Scan (insert new block): %w", err)
				}
				newblocks = append(newblocks, blocknid)
				return nil
			}(); err != nil {
				return err
			}

			var newsnapshot types.StateSnapshotNID
			err = tx.QueryRow(`
				INSERT INTO roomserver_state_snapshots (room_nid, state_block_nids)
					VALUES ($1, $2)
					ON CONFLICT (room_nid, state_block_nids) DO UPDATE SET room_nid=$3
					RETURNING state_snapshot_nid
			`, room, newblocks, room).Scan(&newsnapshot)
			if err != nil {
				return fmt.Errorf("tx.QueryRow.Scan (insert new snapshot): %w", err)
			}

			_, err = tx.Exec(`UPDATE roomserver_events SET state_snapshot_nid=$1 WHERE state_snapshot_nid=$2`, newsnapshot, snapshot)
			if err != nil {
				return fmt.Errorf("tx.Exec (update events): %w", err)
			}
			_, err = tx.Exec(`UPDATE roomserver_rooms SET state_snapshot_nid=$1 WHERE state_snapshot_nid=$2`, newsnapshot, snapshot)
			if err != nil {
				return fmt.Errorf("tx.Exec (update rooms): %w", err)
			}

			fmt.Println("Rewrote snapshot", snapshot, "to", newsnapshot)
		}
	}

	/*
		if _, err = tx.Exec(`DROP TABLE _roomserver_state_snapshots;`); err != nil {
			return fmt.Errorf("tx.Exec (delete old snapshot table): %w", err)
		}
		if _, err = tx.Exec(`DROP TABLE _roomserver_state_block;`); err != nil {
			return fmt.Errorf("tx.Exec (delete old block table): %w", err)
		}
	*/

	return nil
}

func DownStateBlocksRefactor(tx *sql.Tx) error {
	panic("DOWN!")
}
