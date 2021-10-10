package consumers

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Shopify/sarama"
	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/internal/pushgateway"
	"github.com/matrix-org/dendrite/pushserver/api"
	"github.com/matrix-org/dendrite/pushserver/storage"
	rsapi "github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/setup/process"
	"github.com/matrix-org/gomatrixserverlib"
	log "github.com/sirupsen/logrus"
)

type OutputRoomEventConsumer struct {
	cfg        *config.PushServer
	rsAPI      rsapi.RoomserverInternalAPI
	psAPI      api.PushserverInternalAPI
	pgClient   pushgateway.Client
	rsConsumer *internal.ContinualConsumer
	db         storage.Database
}

func NewOutputRoomEventConsumer(
	process *process.ProcessContext,
	cfg *config.PushServer,
	kafkaConsumer sarama.Consumer,
	store storage.Database,
	pgClient pushgateway.Client,
	psAPI api.PushserverInternalAPI,
	rsAPI rsapi.RoomserverInternalAPI,
) *OutputRoomEventConsumer {
	consumer := internal.ContinualConsumer{
		Process:        process,
		ComponentName:  "pushserver/roomserver",
		Topic:          string(cfg.Matrix.Kafka.TopicFor(config.TopicOutputRoomEvent)),
		Consumer:       kafkaConsumer,
		PartitionStore: store,
	}
	s := &OutputRoomEventConsumer{
		cfg:        cfg,
		rsConsumer: &consumer,
		db:         store,
		rsAPI:      rsAPI,
		psAPI:      psAPI,
		pgClient:   pgClient,
	}
	consumer.ProcessMessage = s.onMessage
	return s
}

func (s *OutputRoomEventConsumer) Start() error {
	return s.rsConsumer.Start()
}

func (s *OutputRoomEventConsumer) onMessage(msg *sarama.ConsumerMessage) error {
	ctx := context.Background()

	var output rsapi.OutputEvent
	if err := json.Unmarshal(msg.Value, &output); err != nil {
		log.WithError(err).Errorf("pushserver consumer: message parse failure")
		return nil
	}

	log.WithFields(log.Fields{
		"event_type": output.Type,
	}).Infof("Received message from room server: %#v", output)

	switch output.Type {
	case rsapi.OutputTypeNewRoomEvent:
		ev := output.NewRoomEvent.Event
		if err := s.processMessage(ctx, output.NewRoomEvent.Event); err != nil {
			log.WithFields(log.Fields{
				"event_id": ev.EventID(),
				"event":    string(ev.JSON()),
			}).WithError(err).Errorf("pushserver consumer: process room event failure")
		}

	case rsapi.OutputTypeNewInviteEvent:
		ev := output.NewInviteEvent.Event
		if err := s.processMessage(ctx, output.NewInviteEvent.Event); err != nil {
			log.WithFields(log.Fields{
				"event_id": ev.EventID(),
				"event":    string(ev.JSON()),
			}).WithError(err).Errorf("pushserver consumer: process invite event failure")
		}

	default:
		// Ignore old events, peeks, so on.
	}

	return nil
}

func (s *OutputRoomEventConsumer) processMessage(ctx context.Context, event *gomatrixserverlib.HeaderedEvent) error {
	log.WithFields(log.Fields{
		"event_type": event.Type(),
	}).Infof("Received event from room server: %#v", event)

	members, err := s.localRoomMembers(ctx, event.RoomID())
	if err != nil {
		return err
	}

	log.WithFields(log.Fields{
		"room_id":     event.RoomID(),
		"num_members": len(members),
		"room_size":   roomSize,
	}).Infof("Notifying push gateways")

	// Notification.UserIsTarget is a per-member field, so we
	// cannot group all users in a single request.
	//
	// TODO: does it have to be set? It's not required, and
	// removing it means we can send all notifications to
	// e.g. Element's Push gateway in one go.
	for _, localpart := range members {
		if err := s.notifyLocal(ctx, event, localpart); err != nil {
			log.WithFields(log.Fields{
				"localpart": localpart,
			}).WithError(err).Errorf("Unable to evaluate push rules")
			continue
		}
	}

	return nil
}

// localRoomMembers fetches the current local members of a room.
func (s *OutputRoomEventConsumer) localRoomMembers(ctx context.Context, roomID string) ([]string, error) {
	req := &rsapi.QueryMembershipsForRoomRequest{
		RoomID:     roomID,
		JoinedOnly: true,
	}
	var res rsapi.QueryMembershipsForRoomResponse

	// XXX: This could potentially race if the state for the event is not known yet
	// e.g. the event came over federation but we do not have the full state persisted.
	if err := s.rsAPI.QueryMembershipsForRoom(ctx, req, &res); err != nil {
		return nil, err
	}

	var members []string
	for _, event := range res.JoinEvents {
		if event.StateKey == nil {
			continue
		}

		var member gomatrixserverlib.MemberContent
		if err := json.Unmarshal(event.Content, &member); err != nil {
			log.WithError(err).Errorf("Parsing MemberContent")
			continue
		}
		if member.Membership != gomatrixserverlib.Join {
			continue
		}

		localpart, domain, err := gomatrixserverlib.SplitID('@', *event.StateKey)
		if err != nil {
			log.WithFields(log.Fields{
				"state_key": *event.StateKey,
			}).WithError(err).Errorf("Unable to split MXID")
			continue
		}
		if domain != s.cfg.Matrix.ServerName {
			continue
		}

		members = append(members, localpart)
	}

	return members, nil
}

// notifyLocal finds the right push actions for a local user, given an event.
func (s *OutputRoomEventConsumer) notifyLocal(ctx context.Context, event *gomatrixserverlib.HeaderedEvent, localpart string) error {
	ok, tweaks, err := s.evaluatePushRules(ctx, event, localpart)
	if err != nil {
		return err
	} else if !ok {
		return nil
	}

	devicesByURL, err := s.localPushDevices(ctx, localpart, tweaks)
	if err != nil {
		return err
	}

	log.WithFields(log.Fields{
		"room_id":   event.RoomID(),
		"localpart": localpart,
		"num_urls":  len(devicesByURL),
	}).Infof("Notifying push gateways")

	var rejected []*pushgateway.Device
	for url, devices := range devicesByURL {
		log.WithFields(log.Fields{
			"room_id":   event.RoomID(),
			"localpart": localpart,
			"url":       url,
		}).Infof("Notifying push gateway")

		// TODO: support "email".
		if !strings.HasPrefix(url, "http") {
			continue
		}

		rej, err := s.notifyHTTP(ctx, event, url, devices, localpart)
		if err != nil {
			log.WithFields(log.Fields{
				"event_id":  event.EventID(),
				"localpart": localpart,
			}).WithError(err).Errorf("Unable to notify HTTP pusher")
			continue
		}
		rejected = append(rejected, rej...)
	}

	if len(rejected) > 0 {
		return s.deleteRejectedPushers(ctx, rejected, localpart)
	}

	return nil
}

// evaluatePushRules fetches and evaluates the push rules of a local
// user. Returns true if the event should be pushed.
func (s *OutputRoomEventConsumer) evaluatePushRules(ctx context.Context, event *gomatrixserverlib.HeaderedEvent, localpart string) (ok bool, tweaks map[string]interface{}, err error) {
	// TODO: evaluate push rules
	return true, nil, nil
}

// localPushDevices pushes to the configured devices of a local user.
func (s *OutputRoomEventConsumer) localPushDevices(ctx context.Context, localpart string, tweaks map[string]interface{}) (map[string][]*pushgateway.Device, error) {
	req := &api.QueryPushersRequest{Localpart: localpart}
	var res api.QueryPushersResponse
	if err := s.psAPI.QueryPushers(ctx, req, &res); err != nil {
		return nil, err
	}

	devicesByURL := make(map[string][]*pushgateway.Device, len(res.Pushers))
	for _, pusher := range res.Pushers {
		var url string
		data := pusher.Data
		switch pusher.Kind {
		case api.EmailKind:
			url = "mailto:"

		case api.HTTPKind:
			if format := pusher.Data["format"]; format != nil && format != "event_id_only" {
				log.WithFields(log.Fields{
					"localpart": localpart,
					"app_id":    pusher.AppID,
				}).Errorf("Only data.format event_id_only is supported")
				continue
			}

			urlIface := pusher.Data["url"]
			var ok bool
			url, ok = urlIface.(string)
			if !ok {
				log.WithFields(log.Fields{
					"localpart": localpart,
					"app_id":    pusher.AppID,
				}).Errorf("No data.url configured for HTTP Pusher")
				continue
			}
			data = mapWithout(data, "url")

		default:
			log.WithFields(log.Fields{
				"localpart": localpart,
				"app_id":    pusher.AppID,
				"kind":      pusher.Kind,
			}).Errorf("Unhandled pusher kind")
			continue
		}

		devicesByURL[url] = append(devicesByURL[url], &pushgateway.Device{
			AppID:   pusher.AppID,
			Data:    data,
			PushKey: pusher.PushKey,
			Tweaks:  tweaks,
		})
	}

	return devicesByURL, nil
}

// notifyHTTP performs a notificatation to a Push Gateway.
func (s *OutputRoomEventConsumer) notifyHTTP(ctx context.Context, event *gomatrixserverlib.HeaderedEvent, url string, devices []*pushgateway.Device, localpart string) ([]*pushgateway.Device, error) {
	// This assumes that all devices have format==event_id_only, which
	// is true as long as that's the only allowed format.
	req := &pushgateway.NotifyRequest{
		Notification: pushgateway.Notification{
			Counts:  &pushgateway.Counts{},
			Devices: devices,
			EventID: event.EventID(),
			RoomID:  event.RoomID(),
			Type:    event.Type(),
		},
	}
	if event.StateKey() != nil && *event.StateKey() == fmt.Sprintf("@%s:%s", localpart, s.cfg.Matrix.ServerName) {
		req.Notification.UserIsTarget = true
	}

	log.WithFields(log.Fields{
		"url":         url,
		"localpart":   localpart,
		"app_id0":     devices[0].AppID,
		"num_devices": len(devices),
	}).Infof("Notifying HTTP push gateway")

	var res pushgateway.NotifyResponse
	if err := s.pgClient.Notify(ctx, url, req, &res); err != nil {
		return nil, err
	}

	if len(res.Rejected) == 0 {
		return nil, nil
	}

	devMap := make(map[string]*pushgateway.Device, len(devices))
	for _, d := range devices {
		devMap[d.PushKey] = d
	}
	rejected := make([]*pushgateway.Device, 0, len(res.Rejected))
	for _, pushKey := range res.Rejected {
		d := devMap[pushKey]
		if d != nil {
			rejected = append(rejected, d)
		}
	}

	return rejected, nil
}

// deleteRejectedPushers deletes the pushers associated with the given devices.
func (s *OutputRoomEventConsumer) deleteRejectedPushers(ctx context.Context, devices []*pushgateway.Device, localpart string) error {
	log.WithFields(log.Fields{
		"localpart":   localpart,
		"app_id0":     devices[0].AppID,
		"num_devices": len(devices),
	}).Infof("Deleting pushers rejected by the HTTP push gateway")

	for _, d := range devices {
		if err := s.db.RemovePusher(ctx, d.AppID, d.PushKey, localpart); err != nil {
			log.WithFields(log.Fields{
				"localpart": localpart,
			}).WithError(err).Errorf("Unable to delete rejected pusher")
		}
	}

	return nil
}

// mapWithout returns a shallow copy of the map, without the given
// key. Returns nil if the resulting map is empty.
func mapWithout(m map[string]interface{}, key string) map[string]interface{} {
	ret := make(map[string]interface{}, len(m))
	for k, v := range m {
		// The specification says we do not send "url".
		if k == key {
			continue
		}
		ret[k] = v
	}
	if len(ret) == 0 {
		return nil
	}
	return ret
}
