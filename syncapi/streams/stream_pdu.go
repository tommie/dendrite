package streams

import (
	"context"

	"github.com/matrix-org/dendrite/syncapi/types"
	"github.com/matrix-org/gomatrixserverlib"
)

type PDUStreamProvider struct {
	StreamProvider
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

	if err := p.DB.PDUCompleteSync(ctx, req, joinedRoomIDs, r, &stateFilter, &eventFilter); err != nil {
		req.Log.WithError(err).Error("p.DB.PDUCompleteSync failed")
		return from
	}

	return to
}

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

	if err := p.DB.PDUIncrementalSync(ctx, req, r, from, to); err != nil {
		req.Log.WithError(err).Error("p.DB.PDUIncrementalSync failed")
		return from
	}

	return r.To
}
