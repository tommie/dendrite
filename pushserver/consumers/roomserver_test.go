package consumers

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/matrix-org/dendrite/internal/pushgateway"
	"github.com/matrix-org/dendrite/internal/pushrules"
	"github.com/matrix-org/dendrite/pushserver/api"
	"github.com/matrix-org/dendrite/pushserver/storage"
	rsapi "github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/gomatrixserverlib"
)

const serverName = gomatrixserverlib.ServerName("example.org")

func TestOutputRoomEventConsumer(t *testing.T) {
	ctx := context.Background()

	dbopts := &config.DatabaseOptions{
		ConnectionString:   "file::memory:",
		MaxOpenConnections: 1,
		MaxIdleConnections: 1,
	}
	db, err := storage.Open(dbopts)
	if err != nil {
		t.Fatalf("NewDatabase failed: %v", err)
	}
	var rsAPI fakeRoomServerInternalAPI
	var psAPI fakePushserverInternalAPI
	var wg sync.WaitGroup
	wg.Add(1)
	pgClient := fakePushGatewayClient{
		WG: &wg,
	}
	s := &OutputRoomEventConsumer{
		cfg: &config.PushServer{
			Matrix: &config.Global{
				ServerName: serverName,
			},
		},
		db:       db,
		rsAPI:    &rsAPI,
		psAPI:    &psAPI,
		pgClient: &pgClient,
	}

	event, err := gomatrixserverlib.NewEventFromTrustedJSONWithEventID("$143273582443PhrSn:example.org", []byte(`{
  "content": {
    "body": "This is an example text message",
    "format": "org.matrix.custom.html",
    "formatted_body": "\u003cb\u003eThis is an example text message\u003c/b\u003e",
    "msgtype": "m.text"
  },
  "origin_server_ts": 1432735824653,
  "room_id": "!jEsUZKDJdhlrceRyVU:example.org",
  "sender": "@example:example.org",
  "type": "m.room.message",
  "unsigned": {
    "age": 1234
  }
}`), false, gomatrixserverlib.RoomVersionV7)
	if err != nil {
		t.Fatalf("NewEventFromTrustedJSON failed: %v", err)
	}

	ev := &gomatrixserverlib.HeaderedEvent{
		Event: event,
	}
	if err := s.processMessage(ctx, ev); err != nil {
		t.Fatalf("processMessage failed: %v", err)
	}

	t.Log("Waiting for backend calls to finish.")
	wg.Wait()

	if diff := cmp.Diff([]*rsapi.QueryMembershipsForRoomRequest{{JoinedOnly: true, RoomID: "!jEsUZKDJdhlrceRyVU:example.org"}}, rsAPI.Reqs); diff != "" {
		t.Errorf("rsAPI.QueryMembershipsForRoom Reqs: +got -want:\n%s", diff)
	}
	if diff := cmp.Diff([]*api.QueryPushersRequest{{Localpart: "alice"}}, psAPI.Reqs); diff != "" {
		t.Errorf("psAPI.QueryPushers Reqs: +got -want:\n%s", diff)
	}
	if diff := cmp.Diff([]*pushgateway.NotifyRequest{{
		Notification: pushgateway.Notification{
			Type:    "m.room.message",
			Content: event.Content(),
			Counts:  &pushgateway.Counts{},
			Devices: []*pushgateway.Device{{
				AppID:   "anappid",
				PushKey: "apushkey",
				Data: map[string]interface{}{
					"extra": "someextra",
				},
			}},
			EventID: "$143273582443PhrSn:example.org",
			ID:      "$143273582443PhrSn:example.org",
			RoomID:  "!jEsUZKDJdhlrceRyVU:example.org",
			Sender:  "@example:example.org",
		},
	}}, pgClient.Reqs); diff != "" {
		t.Errorf("pgClient.NotifyHTTP Reqs: +got -want:\n%s", diff)
	}
}

type fakeRoomServerInternalAPI struct {
	rsapi.RoomserverInternalAPI

	Reqs []*rsapi.QueryMembershipsForRoomRequest
}

func (s *fakeRoomServerInternalAPI) QueryMembershipsForRoom(
	ctx context.Context,
	req *rsapi.QueryMembershipsForRoomRequest,
	res *rsapi.QueryMembershipsForRoomResponse,
) error {
	s.Reqs = append(s.Reqs, req)
	*res = rsapi.QueryMembershipsForRoomResponse{
		JoinEvents: []gomatrixserverlib.ClientEvent{
			mustParseClientEventJSON(`{
  "content": {
    "avatar_url": "mxc://example.org/SEsfnsuifSDFSSEF",
    "displayname": "Alice Margatroid",
    "membership": "join",
    "reason": "Looking for support"
  },
  "event_id": "$3957tyerfgewrf384:example.org",
  "origin_server_ts": 1432735824653,
  "room_id": "!jEsUZKDJdhlrceRyVU:example.org",
  "sender": "@example:example.org",
  "state_key": "@alice:example.org",
  "type": "m.room.member",
  "unsigned": {
    "age": 1234
  }
}`),
		},
	}
	return nil
}

type fakePushserverInternalAPI struct {
	api.PushserverInternalAPI

	Reqs []*api.QueryPushersRequest
}

func (s *fakePushserverInternalAPI) QueryPushers(ctx context.Context, req *api.QueryPushersRequest, res *api.QueryPushersResponse) error {
	s.Reqs = append(s.Reqs, req)
	*res = api.QueryPushersResponse{
		Pushers: []api.Pusher{
			{
				PushKey: "apushkey",
				Kind:    api.HTTPKind,
				AppID:   "anappid",
				Data: map[string]interface{}{
					"url":   "http://example.org/pusher/notify",
					"extra": "someextra",
				},
			},
		},
	}
	return nil
}

func (s *fakePushserverInternalAPI) QueryPushRules(ctx context.Context, req *api.QueryPushRulesRequest, res *api.QueryPushRulesResponse) error {
	localpart, _, err := gomatrixserverlib.SplitID('@', req.UserID)
	if err != nil {
		return err
	}
	res.RuleSets = pushrules.DefaultAccountRuleSets(localpart, "example.org")
	return nil
}

type fakePushGatewayClient struct {
	pushgateway.Client

	WG   *sync.WaitGroup
	Reqs []*pushgateway.NotifyRequest
}

func (c *fakePushGatewayClient) Notify(ctx context.Context, url string, req *pushgateway.NotifyRequest, res *pushgateway.NotifyResponse) error {
	c.Reqs = append(c.Reqs, req)
	if c.WG != nil {
		c.WG.Done()
	}
	*res = pushgateway.NotifyResponse{
		Rejected: []string{
			"apushkey",
		},
	}
	return nil
}

func mustMarshalJSON(v interface{}) []byte {
	bs, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return bs
}

func mustParseClientEventJSON(s string) gomatrixserverlib.ClientEvent {
	var ev gomatrixserverlib.ClientEvent
	if err := json.Unmarshal([]byte(s), &ev); err != nil {
		panic(err)
	}
	return ev
}
