package shared

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/syncapi/types"
	userapi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
)

func (d *Database) PDUCompleteSync(
	ctx context.Context,
	req *types.SyncRequest,
	joinedRoomIDs []string,
	r types.Range,
	stateFilter *gomatrixserverlib.StateFilter,
	eventFilter *gomatrixserverlib.RoomEventFilter,
) error {
	txn, err := d.readOnlySnapshot(ctx)
	if err != nil {
		return fmt.Errorf("d.readOnlySnapshot: %w", err)
	}
	var succeeded bool
	defer sqlutil.EndTransactionWithCheck(txn, &succeeded, &err)

	for _, roomID := range joinedRoomIDs {
		jr, err := d.getJoinResponseForCompleteSync(
			ctx, txn, roomID, r, stateFilter, eventFilter, req.WantFullState, req.Device,
		)
		if err != nil {
			return fmt.Errorf("d.getJoinResponseForCompleteSync: %w", err)
		}

		req.Response.Rooms.Join[roomID] = *jr
		req.Rooms[roomID] = gomatrixserverlib.Join
	}

	return nil
}

func (d *Database) PDUIncrementalSync(
	ctx context.Context,
	req *types.SyncRequest,
	r types.Range,
	from, to types.StreamPosition,
) error {
	txn, err := d.readOnlySnapshot(ctx)
	if err != nil {
		return fmt.Errorf("d.readOnlySnapshot: %w", err)
	}
	var succeeded bool
	defer sqlutil.EndTransactionWithCheck(txn, &succeeded, &err)

	var stateDeltas []types.StateDelta
	var joinedRooms []string

	stateFilter := req.Filter.Room.State
	eventFilter := req.Filter.Room.Timeline

	if req.WantFullState {
		if stateDeltas, joinedRooms, err = d.getStateDeltasForFullStateSync(ctx, txn, req.Device, r, req.Device.UserID, &stateFilter); err != nil {
			req.Log.WithError(err).Error("p.DB.GetStateDeltasForFullStateSync failed")
			return fmt.Errorf("d.GetStateDeltasForFullStateSync: %w", err)
		}
	} else {
		if stateDeltas, joinedRooms, err = d.getStateDeltas(ctx, txn, req.Device, r, req.Device.UserID, &stateFilter); err != nil {
			req.Log.WithError(err).Error("p.DB.GetStateDeltas failed")
			return nil
		}
	}

	for _, roomID := range joinedRooms {
		req.Rooms[roomID] = gomatrixserverlib.Join
	}

	for _, delta := range stateDeltas {
		if err = d.addRoomDeltaToResponse(ctx, txn, req.Device, r, delta, &eventFilter, req.Response); err != nil {
			req.Log.WithError(err).Error("d.addRoomDeltaToResponse failed")
			return nil
		}
	}

	return nil
}

func (d *Database) addRoomDeltaToResponse(
	ctx context.Context,
	txn *sql.Tx,
	device *userapi.Device,
	r types.Range,
	delta types.StateDelta,
	eventFilter *gomatrixserverlib.RoomEventFilter,
	res *types.Response,
) error {
	if delta.MembershipPos > 0 && delta.Membership == gomatrixserverlib.Leave {
		// make sure we don't leak recent events after the leave event.
		// TODO: History visibility makes this somewhat complex to handle correctly. For example:
		// TODO: This doesn't work for join -> leave in a single /sync request (see events prior to join).
		// TODO: This will fail on join -> leave -> sensitive msg -> join -> leave
		//       in a single /sync request
		// This is all "okay" assuming history_visibility == "shared" which it is by default.
		r.To = delta.MembershipPos
	}
	recentStreamEvents, limited, err := d.OutputEvents.SelectRecentEvents(
		ctx, txn, delta.RoomID, r,
		eventFilter, true, true,
	)
	if err != nil {
		return err
	}
	recentEvents := d.StreamEventsToEvents(device, recentStreamEvents)
	delta.StateEvents = removeDuplicates(delta.StateEvents, recentEvents) // roll back
	prevBatch, err := d.GetBackwardTopologyPos(ctx, recentStreamEvents)
	if err != nil {
		return err
	}

	// XXX: should we ever get this far if we have no recent events or state in this room?
	// in practice we do for peeks, but possibly not joins?
	if len(recentEvents) == 0 && len(delta.StateEvents) == 0 {
		return nil
	}

	switch delta.Membership {
	case gomatrixserverlib.Join:
		jr := types.NewJoinResponse()
		jr.Timeline.PrevBatch = &prevBatch
		jr.Timeline.Events = gomatrixserverlib.HeaderedToClientEvents(recentEvents, gomatrixserverlib.FormatSync)
		jr.Timeline.Limited = limited
		jr.State.Events = gomatrixserverlib.HeaderedToClientEvents(delta.StateEvents, gomatrixserverlib.FormatSync)
		res.Rooms.Join[delta.RoomID] = *jr

	case gomatrixserverlib.Peek:
		jr := types.NewJoinResponse()
		jr.Timeline.PrevBatch = &prevBatch
		jr.Timeline.Events = gomatrixserverlib.HeaderedToClientEvents(recentEvents, gomatrixserverlib.FormatSync)
		jr.Timeline.Limited = limited
		jr.State.Events = gomatrixserverlib.HeaderedToClientEvents(delta.StateEvents, gomatrixserverlib.FormatSync)
		res.Rooms.Peek[delta.RoomID] = *jr

	case gomatrixserverlib.Leave:
		fallthrough // transitions to leave are the same as ban

	case gomatrixserverlib.Ban:
		// TODO: recentEvents may contain events that this user is not allowed to see because they are
		//       no longer in the room.
		lr := types.NewLeaveResponse()
		lr.Timeline.PrevBatch = &prevBatch
		lr.Timeline.Events = gomatrixserverlib.HeaderedToClientEvents(recentEvents, gomatrixserverlib.FormatSync)
		lr.Timeline.Limited = false // TODO: if len(events) >= numRecents + 1 and then set limited:true
		lr.State.Events = gomatrixserverlib.HeaderedToClientEvents(delta.StateEvents, gomatrixserverlib.FormatSync)
		res.Rooms.Leave[delta.RoomID] = *lr
	}

	return nil
}

func (d *Database) getJoinResponseForCompleteSync(
	ctx context.Context,
	txn *sql.Tx,
	roomID string,
	r types.Range,
	stateFilter *gomatrixserverlib.StateFilter,
	eventFilter *gomatrixserverlib.RoomEventFilter,
	wantFullState bool,
	device *userapi.Device,
) (jr *types.JoinResponse, err error) {
	// TODO: When filters are added, we may need to call this multiple times to get enough events.
	//       See: https://github.com/matrix-org/synapse/blob/v0.19.3/synapse/handlers/sync.py#L316
	recentStreamEvents, limited, err := d.OutputEvents.SelectRecentEvents(
		ctx, txn, roomID, r, eventFilter, true, true,
	)
	if err != nil {
		return
	}

	// Get the event IDs of the stream events we fetched. There's no point in us
	var excludingEventIDs []string
	if !wantFullState {
		excludingEventIDs = make([]string, 0, len(recentStreamEvents))
		for _, event := range recentStreamEvents {
			if event.StateKey() != nil {
				excludingEventIDs = append(excludingEventIDs, event.EventID())
			}
		}
	}

	stateEvents, err := d.CurrentRoomState.SelectCurrentState(ctx, txn, roomID, stateFilter, excludingEventIDs)
	if err != nil {
		return
	}

	// TODO FIXME: We don't fully implement history visibility yet. To avoid leaking events which the
	// user shouldn't see, we check the recent events and remove any prior to the join event of the user
	// which is equiv to history_visibility: joined
	joinEventIndex := -1
	for i := len(recentStreamEvents) - 1; i >= 0; i-- {
		ev := recentStreamEvents[i]
		if ev.Type() == gomatrixserverlib.MRoomMember && ev.StateKeyEquals(device.UserID) {
			membership, _ := ev.Membership()
			if membership == "join" {
				joinEventIndex = i
				if i > 0 {
					// the create event happens before the first join, so we should cut it at that point instead
					if recentStreamEvents[i-1].Type() == gomatrixserverlib.MRoomCreate && recentStreamEvents[i-1].StateKeyEquals("") {
						joinEventIndex = i - 1
						break
					}
				}
				break
			}
		}
	}
	if joinEventIndex != -1 {
		// cut all events earlier than the join (but not the join itself)
		recentStreamEvents = recentStreamEvents[joinEventIndex:]
		limited = false // so clients know not to try to backpaginate
	}

	// Retrieve the backward topology position, i.e. the position of the
	// oldest event in the room's topology.
	var prevBatch *types.TopologyToken
	if len(recentStreamEvents) > 0 {
		var backwardTopologyPos, backwardStreamPos types.StreamPosition
		backwardTopologyPos, backwardStreamPos, err = d.PositionInTopology(ctx, recentStreamEvents[0].EventID())
		if err != nil {
			return
		}
		prevBatch = &types.TopologyToken{
			Depth:       backwardTopologyPos,
			PDUPosition: backwardStreamPos,
		}
		prevBatch.Decrement()
	}

	// We don't include a device here as we don't need to send down
	// transaction IDs for complete syncs, but we do it anyway because Sytest demands it for:
	// "Can sync a room with a message with a transaction id" - which does a complete sync to check.
	recentEvents := d.StreamEventsToEvents(device, recentStreamEvents)
	stateEvents = removeDuplicates(stateEvents, recentEvents)
	jr = types.NewJoinResponse()
	jr.Timeline.PrevBatch = prevBatch
	jr.Timeline.Events = gomatrixserverlib.HeaderedToClientEvents(recentEvents, gomatrixserverlib.FormatSync)
	jr.Timeline.Limited = limited
	jr.State.Events = gomatrixserverlib.HeaderedToClientEvents(stateEvents, gomatrixserverlib.FormatSync)
	return jr, nil
}

func removeDuplicates(stateEvents, recentEvents []*gomatrixserverlib.HeaderedEvent) []*gomatrixserverlib.HeaderedEvent {
	for _, recentEv := range recentEvents {
		if recentEv.StateKey() == nil {
			continue // not a state event
		}
		// TODO: This is a linear scan over all the current state events in this room. This will
		//       be slow for big rooms. We should instead sort the state events by event ID  (ORDER BY)
		//       then do a binary search to find matching events, similar to what roomserver does.
		for j := 0; j < len(stateEvents); j++ {
			if stateEvents[j].EventID() == recentEv.EventID() {
				// overwrite the element to remove with the last element then pop the last element.
				// This is orders of magnitude faster than re-slicing, but doesn't preserve ordering
				// (we don't care about the order of stateEvents)
				stateEvents[j] = stateEvents[len(stateEvents)-1]
				stateEvents = stateEvents[:len(stateEvents)-1]
				break // there shouldn't be multiple events with the same event ID
			}
		}
	}
	return stateEvents
}
