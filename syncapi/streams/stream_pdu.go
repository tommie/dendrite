package streams

import (
	"context"
	"encoding/json"
	"fmt"

	rsapi "github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/syncapi/types"
	userapi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
)

type PDUStreamProvider struct {
	StreamProvider
	rsAPI rsapi.RoomserverInternalAPI
}

func (p *PDUStreamProvider) Setup() {
	p.StreamProvider.Setup()

	p.latestMutex.Lock()
	defer p.latestMutex.Unlock()

	id, err := p.DB.MaxStreamPositionForPDUs(context.Background())
	if err != nil {
		panic(err)
	}
	p.latest = id
}

func (p *PDUStreamProvider) CompleteSync(
	ctx context.Context,
	req *types.SyncRequest,
) types.StreamPosition {
	from := types.StreamPosition(0)
	to := p.LatestPosition(ctx)

	// Get the current sync position which we will base the sync response on.
	// For complete syncs, we want to start at the most recent events and work
	// backwards, so that we show the most recent events in the room.
	r := types.Range{
		From:      to,
		To:        0,
		Backwards: true,
	}

	// Extract room state and recent events for all rooms the user is joined to.
	joinedRoomIDs, err := p.DB.RoomIDsWithMembership(ctx, req.Device.UserID, gomatrixserverlib.Join)
	if err != nil {
		req.Log.WithError(err).Error("p.DB.RoomIDsWithMembership failed")
		return from
	}

	stateFilter := req.Filter.Room.State
	eventFilter := req.Filter.Room.Timeline

	// Build up a /sync response. Add joined rooms.
	for _, roomID := range joinedRoomIDs {
		var jr *types.JoinResponse
		jr, err = p.getJoinResponseForCompleteSync(
			ctx, roomID, r, &stateFilter, &eventFilter, req.Device,
		)
		if err != nil {
			req.Log.WithError(err).Error("p.getJoinResponseForCompleteSync failed")
			return from
		}
		req.Response.Rooms.Join[roomID] = *jr
		req.Rooms[roomID] = gomatrixserverlib.Join
	}

	// Add peeked rooms.
	peeks, err := p.DB.PeeksInRange(ctx, req.Device.UserID, req.Device.ID, r)
	if err != nil {
		req.Log.WithError(err).Error("p.DB.PeeksInRange failed")
		return from
	}
	for _, peek := range peeks {
		if !peek.Deleted {
			var jr *types.JoinResponse
			jr, err = p.getJoinResponseForCompleteSync(
				ctx, peek.RoomID, r, &stateFilter, &eventFilter, req.Device,
			)
			if err != nil {
				req.Log.WithError(err).Error("p.getJoinResponseForCompleteSync failed")
				return from
			}
			req.Response.Rooms.Peek[peek.RoomID] = *jr
		}
	}

	if req.Filter.Room.IncludeLeave {
		var leaveRoomIDs []string
		// Extract room state and recent events for all rooms the user has left
		leaveRoomIDs, err := p.DB.RoomIDsWithMembership(ctx, req.Device.UserID, gomatrixserverlib.Leave)
		if err != nil {
			req.Log.WithError(err).Error("p.DB.RoomIDsWithMembership failed")
			return from
		}
		// Build up a /sync response. Add leave rooms.
		for _, roomID := range leaveRoomIDs {
			var lr *types.LeaveResponse
			lr, err = p.getLeaveResponseForCompleteSync(
				ctx, roomID, r, &stateFilter, &eventFilter, req.Device,
			)
			if err != nil {
				req.Log.WithError(err).Error("p.getLeaveResponseForCompleteSync failed")
				return from
			}
			req.Response.Rooms.Leave[roomID] = *lr
		}
	}

	return to
}

// nolint:gocyclo
func (p *PDUStreamProvider) IncrementalSync(
	ctx context.Context,
	req *types.SyncRequest,
	from, to types.StreamPosition,
) (newPos types.StreamPosition) {
	r := types.Range{
		From:      from,
		To:        to,
		Backwards: from > to,
	}
	newPos = to

	var err error
	var stateDeltas []types.StateDelta
	var joinedRooms []string

	stateFilter := req.Filter.Room.State
	eventFilter := req.Filter.Room.Timeline

	if req.WantFullState {
		if stateDeltas, joinedRooms, err = p.DB.GetStateDeltasForFullStateSync(ctx, req.Device, r, req.Device.UserID, &stateFilter); err != nil {
			req.Log.WithError(err).Error("p.DB.GetStateDeltasForFullStateSync failed")
			return
		}
	} else {
		if stateDeltas, joinedRooms, err = p.DB.GetStateDeltas(ctx, req.Device, r, req.Device.UserID, &stateFilter); err != nil {
			req.Log.WithError(err).Error("p.DB.GetStateDeltas failed")
			return
		}
	}

	for _, roomID := range joinedRooms {
		req.Rooms[roomID] = gomatrixserverlib.Join
	}

	for _, delta := range stateDeltas {
		if err = p.addRoomDeltaToResponse(ctx, req.Device, r, delta, &stateFilter, &eventFilter, req.Response); err != nil {
			req.Log.WithError(err).Error("d.addRoomDeltaToResponse failed")
			return newPos
		}
	}

	return r.To
}

func (p *PDUStreamProvider) getHistoryVisibility(
	ctx context.Context,
	roomID string,
) (string, string, error) {
	historyVisibility := "shared"
	historyEventID := ""

	historyVisFilter := gomatrixserverlib.DefaultStateFilter()
	historyVisFilter.Types = []string{"m.room.history_visibility"}

	historyVisEvents, err := p.DB.CurrentState(ctx, roomID, &historyVisFilter)
	if err != nil {
		return "", "", fmt.Errorf("p.DB.CurrentState: %w", err)
	}

	for _, event := range historyVisEvents {
		if event.Type() != gomatrixserverlib.MRoomHistoryVisibility {
			continue
		}
		var content struct {
			HistoryVisibility string `json:"history_visibility"`
		}
		if err := json.Unmarshal(event.Content(), &content); err != nil {
			return historyVisibility, event.EventID(), fmt.Errorf("json.Unmarshal: %w", err)
		} else {
			historyVisibility = content.HistoryVisibility
			historyEventID = event.EventID()
		}
		break
	}

	return historyVisibility, historyEventID, nil
}

func (p *PDUStreamProvider) addRoomDeltaToResponse(
	ctx context.Context,
	device *userapi.Device,
	r types.Range,
	delta types.StateDelta,
	_ *gomatrixserverlib.StateFilter,
	eventFilter *gomatrixserverlib.RoomEventFilter,
	res *types.Response,
) error {
	historyVisibility, historyEventID, err := p.getHistoryVisibility(ctx, delta.RoomID)
	if err != nil {
		return fmt.Errorf("p.getHistoryVisibility: %w", err)
	}

	if r, _, err = p.limitBoundariesUsingHistoryVisibility(
		ctx, delta.RoomID, device.UserID, historyVisibility, historyEventID, r,
	); err != nil {
		return err
	}

	recentStreamEvents, limited, err := p.DB.RecentEvents(
		ctx, delta.RoomID, r,
		eventFilter, true, true,
	)
	if err != nil {
		return err
	}
	recentEvents := p.DB.StreamEventsToEvents(device, recentStreamEvents)
	delta.StateEvents = removeDuplicates(delta.StateEvents, recentEvents) // roll back
	prevBatch, err := p.DB.GetBackwardTopologyPos(ctx, recentStreamEvents)
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

func (p *PDUStreamProvider) limitBoundariesUsingHistoryVisibility(
	ctx context.Context,
	roomID, userID string,
	historyVisibility, historyEventID string,
	r types.Range,
) (types.Range, string, error) {
	// Calculate the current history visibility rule.

	var err error
	var joinPos types.StreamPosition
	var stateAtEventID string

	// Check and see if the user is in the room.
	switch historyVisibility {
	case "invited", "joined", "shared":
		// Get the most recent membership event of the user and check if
		// they are still in the room. If not then we will restrict how
		// much of the room the user can see - they won't see beyond their
		// leave event.
		joinTypes := []string{"join"}
		if historyVisibility == "invited" {
			joinTypes = []string{"invite", "join"}
		}
		if _, joinPos, _, err = p.DB.MostRecentMembership(ctx, roomID, userID, joinTypes); err != nil {
			// The user isn't a part of the room, or hasn't been invited
			// to the room.
			return r, stateAtEventID, nil
		}

	case "world_readable":
		// It doesn't matter if the user is joined to the room or not
		// when the history is world_readable.
	}

	// If the user is in the room then we next need to work out if we
	// should bind the beginning of the window based on the join position,
	// or the position of the history visibility event.
	switch historyVisibility {
	case "invited", "joined":
		if r.Backwards {
			if r.To > joinPos {
				r.To = joinPos
			}
		} else {
			if r.From > joinPos {
				r.From = joinPos
			}
		}

	case "shared":
		// Find the stream position of the history visibility event
		// and use that as a boundary instead.
		var historyVisibilityPosition types.StreamPosition
		historyVisibilityPosition, err = p.DB.EventPositionInStream(ctx, historyEventID)
		if err != nil {
			return r, stateAtEventID, fmt.Errorf("p.DB.EventPositionInStream: %w", err)
		}
		if r.Backwards {
			if r.To < historyVisibilityPosition {
				r.To = historyVisibilityPosition
			}
		} else {
			if r.From < historyVisibilityPosition {
				r.From = historyVisibilityPosition
			}
		}
		stateAtEventID = historyEventID

	case "world_readable":
		// Do nothing, as it's OK to reveal the entire timeline in a
		// world-readable room.
	}

	// Finally, work out if the user left the room. If they did then
	// we will request the state at the leave event from the roomserver.
	switch historyVisibility {
	case "invited", "joined", "shared":
		if leaveEvent, leavePos, _, err := p.DB.MostRecentMembership(ctx, roomID, userID, []string{"leave", "ban", "kick"}); err == nil {
			if r.Backwards {
				if r.From > leavePos {
					r.From = leavePos
				}
			} else {
				if r.To > leavePos {
					r.To = leavePos
				}
			}
			stateAtEventID = leaveEvent.EventID()
		}

	case "world_readable":
	}

	return r, stateAtEventID, nil
}

// nolint:gocyclo
func (p *PDUStreamProvider) getResponseForCompleteSync(
	ctx context.Context,
	roomID string,
	r types.Range,
	stateFilter *gomatrixserverlib.StateFilter,
	eventFilter *gomatrixserverlib.RoomEventFilter,
	device *userapi.Device,
) (
	recentEvents, stateEvents []*gomatrixserverlib.HeaderedEvent,
	prevBatch *types.TopologyToken, limited bool, err error,
) {
	historyVisibility, historyEventID, err := p.getHistoryVisibility(ctx, roomID)
	if err != nil {
		return
	}

	var stateAtEvent string
	if r, stateAtEvent, err = p.limitBoundariesUsingHistoryVisibility(
		ctx, roomID, device.UserID, historyVisibility, historyEventID, r,
	); err != nil {
		return
	}

	if stateAtEvent == "" {
		stateEvents, err = p.DB.CurrentState(ctx, roomID, stateFilter)
		if err != nil {
			return
		}
	} else {
		queryReq := &rsapi.QueryStateAfterEventsRequest{
			RoomID:       roomID,
			PrevEventIDs: []string{stateAtEvent},
		}
		queryRes := &rsapi.QueryStateAfterEventsResponse{}
		if err = p.rsAPI.QueryStateAfterEvents(ctx, queryReq, queryRes); err != nil {
			return
		}
		stateEvents = p.filterStateEventsAccordingToFilter(queryRes.StateEvents, stateFilter)
	}

	// TODO: When filters are added, we may need to call this multiple times to get enough events.
	//       See: https://github.com/matrix-org/synapse/blob/v0.19.3/synapse/handlers/sync.py#L316
	var recentStreamEvents []types.StreamEvent
	recentStreamEvents, limited, err = p.DB.RecentEvents(
		ctx, roomID, r, eventFilter, true, true,
	)
	if err != nil {
		return
	}

	for _, event := range recentStreamEvents {
		if event.HeaderedEvent.Event.StateKey() != nil {
			stateEvents = append(stateEvents, event.HeaderedEvent)
		}
	}

	// Retrieve the backward topology position, i.e. the position of the
	// oldest event in the room's topology.
	if len(recentStreamEvents) > 0 {
		var backwardTopologyPos, backwardStreamPos types.StreamPosition
		backwardTopologyPos, backwardStreamPos, err = p.DB.PositionInTopology(ctx, recentStreamEvents[0].EventID())
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
	recentEvents = p.DB.StreamEventsToEvents(device, recentStreamEvents)
	stateEvents = removeDuplicates(stateEvents, recentEvents)
	return // nolint:nakedret
}

func (p *PDUStreamProvider) getJoinResponseForCompleteSync(
	ctx context.Context,
	roomID string,
	r types.Range,
	stateFilter *gomatrixserverlib.StateFilter,
	eventFilter *gomatrixserverlib.RoomEventFilter,
	device *userapi.Device,
) (jr *types.JoinResponse, err error) {
	recentEvents, stateEvents, prevBatch, limited, err := p.getResponseForCompleteSync(
		ctx, roomID, r, stateFilter, eventFilter, device,
	)
	if err != nil {
		return nil, fmt.Errorf("p.getResponseForCompleteSync: %w", err)
	}

	jr = types.NewJoinResponse()
	jr.Timeline.PrevBatch = prevBatch
	jr.Timeline.Events = gomatrixserverlib.HeaderedToClientEvents(recentEvents, gomatrixserverlib.FormatSync)
	jr.Timeline.Limited = limited
	jr.State.Events = gomatrixserverlib.HeaderedToClientEvents(stateEvents, gomatrixserverlib.FormatSync)
	return jr, nil
}

func (p *PDUStreamProvider) getLeaveResponseForCompleteSync(
	ctx context.Context,
	roomID string,
	r types.Range,
	stateFilter *gomatrixserverlib.StateFilter,
	eventFilter *gomatrixserverlib.RoomEventFilter,
	device *userapi.Device,
) (lr *types.LeaveResponse, err error) {
	recentEvents, stateEvents, prevBatch, limited, err := p.getResponseForCompleteSync(
		ctx, roomID, r, stateFilter, eventFilter, device,
	)
	if err != nil {
		return nil, fmt.Errorf("p.getResponseForCompleteSync: %w", err)
	}

	lr = types.NewLeaveResponse()
	lr.Timeline.PrevBatch = prevBatch
	lr.Timeline.Events = gomatrixserverlib.HeaderedToClientEvents(recentEvents, gomatrixserverlib.FormatSync)
	lr.Timeline.Limited = limited
	lr.State.Events = gomatrixserverlib.HeaderedToClientEvents(stateEvents, gomatrixserverlib.FormatSync)
	return lr, nil
}

// nolint:gocyclo
func (p *PDUStreamProvider) filterStateEventsAccordingToFilter(
	stateEvents []*gomatrixserverlib.HeaderedEvent,
	stateFilter *gomatrixserverlib.StateFilter,
) []*gomatrixserverlib.HeaderedEvent {
	filterRooms, filterNotRooms := map[string]struct{}{}, map[string]struct{}{}
	filterTypes, filterNotTypes := map[string]struct{}{}, map[string]struct{}{}
	for _, r := range stateFilter.Rooms {
		filterRooms[r] = struct{}{}
	}
	for _, r := range stateFilter.NotRooms {
		filterNotRooms[r] = struct{}{}
	}
	for _, t := range stateFilter.Types {
		filterTypes[t] = struct{}{}
	}
	for _, t := range stateFilter.NotTypes {
		filterNotTypes[t] = struct{}{}
	}

	newState := make([]*gomatrixserverlib.HeaderedEvent, 0, len(stateEvents))
	for _, event := range stateEvents {
		if len(filterRooms) > 0 {
			if _, ok := filterRooms[event.RoomID()]; !ok {
				continue
			}
		}
		if len(filterNotRooms) > 0 {
			if _, ok := filterNotRooms[event.RoomID()]; ok {
				continue
			}
		}
		if len(filterTypes) > 0 {
			if _, ok := filterTypes[event.Type()]; !ok {
				continue
			}
		}
		if len(filterNotTypes) > 0 {
			if _, ok := filterNotTypes[event.Type()]; ok {
				continue
			}
		}
		newState = append(newState, event)
	}

	return newState
}

func removeDuplicates(stateEvents, recentEvents []*gomatrixserverlib.HeaderedEvent) []*gomatrixserverlib.HeaderedEvent {
	timeline := map[string]struct{}{}
	for _, event := range recentEvents {
		if event.StateKey() == nil {
			continue
		}
		timeline[event.EventID()] = struct{}{}
	}
	state := []*gomatrixserverlib.HeaderedEvent{}
	for _, event := range stateEvents {
		if _, ok := timeline[event.EventID()]; ok {
			continue
		}
		state = append(state, event)
	}
	return state
}
