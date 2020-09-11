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

package internal

import (
	"context"

	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
	"github.com/sirupsen/logrus"
)

type queryState interface {
	QueryStateAfterEvents(
		ctx context.Context,
		request *api.QueryStateAfterEventsRequest,
		response *api.QueryStateAfterEventsResponse,
	) error
	QueryMembershipForUser(
		ctx context.Context,
		request *api.QueryMembershipForUserRequest,
		response *api.QueryMembershipForUserResponse,
	) error
}

// ApplyHistoryVisibilityChecks removes items from the input slice if the user is not allowed
// to see these events.
//
// This works by using QueryStateAfterEvents to pull out the history visibility and membership
// events for the user at the time of the each input event, then applying the checks detailled
// at https://matrix.org/docs/spec/client_server/r0.6.0#id87
func ApplyHistoryVisibilityChecks(
	ctx context.Context, rsAPI queryState, userID string, events []gomatrixserverlib.HeaderedEvent,
) []gomatrixserverlib.HeaderedEvent {
	result := make([]gomatrixserverlib.HeaderedEvent, 0, len(events))
	currentMemberships := make(map[string]*api.QueryMembershipForUserResponse) // room_id => membership
	for _, ev := range events {
		queryMembership := currentMemberships[ev.RoomID()]
		if queryMembership == nil {
			var queryRes api.QueryMembershipForUserResponse
			// discard errors, we may not actually need to know their *CURRENT* membership to allow
			// them access, so let's continue rather than giving up.
			_ = rsAPI.QueryMembershipForUser(ctx, &api.QueryMembershipForUserRequest{
				RoomID: ev.RoomID(),
				UserID: userID,
			}, &queryRes)
			currentMemberships[ev.RoomID()] = &queryRes
			queryMembership = &queryRes
		}
		currentMembership := "leave"
		if queryMembership != nil {
			currentMembership = queryMembership.Membership
		}
		if userAllowedToSeeEvent(ctx, rsAPI, userID, currentMembership, ev) {
			result = append(result, ev)
		}
	}
	return result
}

func userAllowedToSeeEvent(ctx context.Context, rsAPI queryState, userID, currentMembership string, ev gomatrixserverlib.HeaderedEvent) bool {
	logger := util.GetLogger(ctx).WithFields(logrus.Fields{
		"requester": userID,
		"room_id":   ev.RoomID(),
		"event_id":  ev.EventID(),
	})
	var queryRes api.QueryStateAfterEventsResponse
	err := rsAPI.QueryStateAfterEvents(ctx, &api.QueryStateAfterEventsRequest{
		RoomID:       ev.RoomID(),
		PrevEventIDs: ev.PrevEventIDs(),
		StateToFetch: []gomatrixserverlib.StateKeyTuple{
			{
				EventType: gomatrixserverlib.MRoomMember,
				StateKey:  userID,
			},
			{
				EventType: gomatrixserverlib.MRoomHistoryVisibility,
				StateKey:  "",
			},
		},
	}, &queryRes)
	if err != nil {
		logger.WithError(err).Error("Failed to lookup state of room at event, denying user access to event")
		return false
	}
	if !queryRes.RoomExists {
		logger.Error("Unknown room, denying user access to event")
		return false
	}
	if !queryRes.PrevEventsExist {
		logger.Error("Failed to lookup state of room at event: missing prev_events for this event, denying user access to event")
		return false
	}
	var membershipEvent, hisVisEvent *gomatrixserverlib.HeaderedEvent
	for i := range queryRes.StateEvents {
		switch queryRes.StateEvents[i].Type() {
		case gomatrixserverlib.MRoomMember:
			membershipEvent = &queryRes.StateEvents[i]
		case gomatrixserverlib.MRoomHistoryVisibility:
			hisVisEvent = &queryRes.StateEvents[i]
		}
	}
	// if they recently joined the room and are requesting events from a long time ago then we expect no membership event
	// so default to leave
	membership := gomatrixserverlib.Leave
	if membershipEvent != nil {
		membership, _ = membershipEvent.Membership()
	}
	return historyVisibilityCheckForEvent(&ev, membership, currentMembership, hisVisEvent)
}

// Implements https://matrix.org/docs/spec/client_server/r0.6.0#id87 for clients. Not to be confused with a similar
// function in roomserver which is designed for servers.
func historyVisibilityCheckForEvent(
	ev *gomatrixserverlib.HeaderedEvent, membership, currentMembership string, hisVisEvent *gomatrixserverlib.HeaderedEvent,
) bool {
	// By default if no history_visibility is set, or if the value is not understood, the visibility is assumed to be shared.
	visibility := "shared"
	knownStates := []string{"invited", "joined", "shared", "world_readable"}
	if hisVisEvent != nil {
		// ignore errors as that means "the value is not understood".
		vis, _ := hisVisEvent.HistoryVisibility()
		for _, knownVis := range knownStates {
			if vis == knownVis {
				visibility = vis
				break
			}
		}
	}

	// 1. If the history_visibility was set to world_readable, allow.
	if visibility == "world_readable" {
		return true
	}
	// 2. If the user's membership was join, allow.
	if membership == "join" {
		return true
	}
	// 3. If history_visibility was set to shared, and the user joined the room at any point after the event was sent, allow.
	// TODO: This is actually an annoying calculation to do. We happen to know that these checks are for /messages which
	//       is usually called whilst you're a room member, so we cheat a bit here by checking if we are currently joined.
	//       This will miss cases where you are not in the room when the event is sent, then join then leave then hit /messages.
	//       This is a slightly more conservative solution. We could alternatively use QueryMembershipForUserResponse.HasBeenInRoom
	//       but this would mean a left user could see NEW messages in a room if the history visibility was set to 'shared', without
	//       having to join it (which would make many people upset, particularly IRC folks).
	if visibility == "shared" && (membership == "join" || currentMembership == "join") {
		return true
	}

	// 4. If the user's membership was invite, and the history_visibility was set to invited, allow.
	if membership == "invite" && visibility == "invited" {
		return true
	}

	return false
}
