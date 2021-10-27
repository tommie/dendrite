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
	"github.com/matrix-org/gomatrixserverlib"
	log "github.com/sirupsen/logrus"
)

type notificationsStatements struct {
	insertStmt      *sql.Stmt
	deleteUpToStmt  *sql.Stmt
	updateReadStmt  *sql.Stmt
	selectStmt      *sql.Stmt
	selectCountStmt *sql.Stmt
}

func prepareNotificationsTable(db *sql.DB) (tables.Notifications, error) {
	s := &notificationsStatements{}

	return s, sqlutil.StatementList{
		{&s.insertStmt, insertNotificationSQL},
		{&s.deleteUpToStmt, deleteNotificationsUpToSQL},
		{&s.updateReadStmt, updateNotificationReadSQL},
		{&s.selectStmt, selectNotificationSQL},
		{&s.selectCountStmt, selectNotificationCountSQL},
	}.Prepare(db)
}

const insertNotificationSQL = "INSERT INTO pushserver_notifications (localpart, room_id, event_id, ts_ms, highlight, notification_json) VALUES ($1, $2, $3, $4, $5, $6)"

// Insert inserts a notification into the database.
func (s *notificationsStatements) Insert(ctx context.Context, localpart, eventID string, highlight bool, n *api.Notification) error {
	roomID, tsMS := n.RoomID, n.TS
	nn := *n
	// Clears out fields that have their own columns to (1) shrink the
	// data and (2) avoid difficult-to-debug inconsistency bugs.
	nn.RoomID = ""
	nn.TS, nn.Read = 0, false
	bs, err := json.Marshal(nn)
	if err != nil {
		return err
	}
	_, err = s.insertStmt.ExecContext(ctx, localpart, roomID, eventID, tsMS, highlight, string(bs))
	return err
}

const deleteNotificationsUpToSQL = `DELETE FROM pushserver_notifications
WHERE
  localpart = $1 AND
  room_id = $2 AND
  id <= (
    SELECT MAX(id)
    FROM pushserver_notifications
    WHERE
      localpart = $1 AND
      room_id = $2 AND
      event_id = $3
  )`

// DeleteUpTo deletes all previous notifications, up to and including the event.
func (s *notificationsStatements) DeleteUpTo(ctx context.Context, localpart, roomID, eventID string) error {
	res, err := s.updateReadStmt.ExecContext(ctx, localpart, roomID, eventID)
	if err != nil {
		return err
	}
	if nrows, err := res.RowsAffected(); err == nil {
		log.WithFields(log.Fields{"localpart": localpart, "room_id": roomID, "event_id": eventID}).Tracef("DeleteUpTo: %d rows affected", nrows)
	}
	return nil
}

const updateNotificationReadSQL = `UPDATE pushserver_notifications
SET read = $1
WHERE
  localpart = $2 AND
  room_id = $3 AND
  id <= (
    SELECT MAX(id)
    FROM pushserver_notifications
    WHERE
      localpart = $2 AND
      room_id = $3 AND
      event_id = $4
  )`

// UpdateRead updates the "read" value for an event.
func (s *notificationsStatements) UpdateRead(ctx context.Context, localpart, roomID, eventID string, v bool) error {
	res, err := s.updateReadStmt.ExecContext(ctx, v, localpart, roomID, eventID)
	if err != nil {
		return err
	}
	if nrows, err := res.RowsAffected(); err == nil {
		log.WithFields(log.Fields{"localpart": localpart, "room_id": roomID, "event_id": eventID}).Tracef("UpdateRead: %d rows affected", nrows)
	}
	return nil
}

const selectNotificationSQL = `SELECT id, room_id, ts_ms, read, notification_json
FROM pushserver_notifications
WHERE
  localpart = $1 AND
  id > $2 AND
  (
    (($3 & 1) <> 0 AND highlight) OR
    (($3 & 2) <> 0 AND NOT highlight)
  ) AND
  NOT read
ORDER BY localpart, id
LIMIT $4`

func (s *notificationsStatements) Select(ctx context.Context, localpart string, fromID int64, limit int, filter tables.NotificationFilter) ([]*api.Notification, int64, error) {
	rows, err := s.selectStmt.QueryContext(ctx, localpart, fromID, uint32(filter), limit)

	if err != nil {
		return nil, 0, err
	}
	defer internal.CloseAndLogIfError(ctx, rows, "notifications.Select: rows.Close() failed")

	var maxID int64 = -1
	var notifs []*api.Notification
	for rows.Next() {
		var id int64
		var roomID string
		var ts gomatrixserverlib.Timestamp
		var read bool
		var jsonStr string
		err = rows.Scan(
			&id,
			&roomID,
			&ts,
			&read,
			&jsonStr)
		if err != nil {
			return nil, 0, err
		}

		var n api.Notification
		err := json.Unmarshal([]byte(jsonStr), &n)
		if err != nil {
			return nil, 0, err
		}
		n.RoomID = roomID
		n.TS = ts
		n.Read = read
		notifs = append(notifs, &n)

		if maxID < id {
			maxID = id
		}
	}
	return notifs, maxID, rows.Err()
}

const selectNotificationCountSQL = `SELECT COUNT(*)
FROM pushserver_notifications
WHERE
  localpart = $1 AND
  (
    (($2 & 1) <> 0 AND highlight) OR
    (($2 & 2) <> 0 AND NOT highlight)
  ) AND
  NOT read`

func (s *notificationsStatements) SelectCount(ctx context.Context, localpart string, filter tables.NotificationFilter) (int64, error) {
	rows, err := s.selectCountStmt.QueryContext(ctx, localpart, uint32(filter))

	if err != nil {
		return 0, err
	}
	defer internal.CloseAndLogIfError(ctx, rows, "notifications.Select: rows.Close() failed")

	for rows.Next() {
		var count int64
		if err := rows.Scan(&count); err != nil {
			return 0, err
		}

		return count, nil
	}
	return 0, rows.Err()
}
