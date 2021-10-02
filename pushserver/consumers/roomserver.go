package consumers

import (
	"context"
	"encoding/json"

	"github.com/Shopify/sarama"
	"github.com/matrix-org/dendrite/internal"
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
	rsConsumer *internal.ContinualConsumer
	db         storage.Database
}

func NewOutputRoomEventConsumer(
	process *process.ProcessContext,
	cfg *config.PushServer,
	kafkaConsumer sarama.Consumer,
	store storage.Database,
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
	}
	consumer.ProcessMessage = s.onMessage
	return s
}

func (s *OutputRoomEventConsumer) Start() error {
	return s.rsConsumer.Start()
}

func (s *OutputRoomEventConsumer) onMessage(msg *sarama.ConsumerMessage) error {
	var output rsapi.OutputEvent
	if err := json.Unmarshal(msg.Value, &output); err != nil {
		log.WithError(err).Errorf("roomserver output log: message parse failure")
		return nil
	}

	switch output.Type {
	case rsapi.OutputTypeNewRoomEvent:
		ev := output.NewRoomEvent.Event
		if err := s.processMessage(*output.NewRoomEvent); err != nil {
			// panic rather than continue with an inconsistent database
			log.WithFields(log.Fields{
				"event_id":   ev.EventID(),
				"event":      string(ev.JSON()),
				log.ErrorKey: err,
			}).Panicf("roomserver output log: write room event failure")
			return err
		}

	default:
		// Ignore old events, peeks, so on.
	}

	return nil
}

var ranAlready = false

func (s *OutputRoomEventConsumer) processMessage(ore rsapi.OutputNewRoomEvent) error {
	// TODO: New events from the roomserver will be passed here.
	event := ore.Event

	if event.Type() == "m.room.message" && !ranAlready {
		ranAlready = true
		log.Debug("🙌🙌🙌 Winning !!!")
		membershipReq := &rsapi.QueryMembershipsForRoomRequest{
			RoomID:     event.RoomID(),
			JoinedOnly: true,
		}
		membershipRes := &rsapi.QueryMembershipsForRoomResponse{}

		// XXX: This could potentially race if the state for the event is not known yet
		// e.g. the event came over federation but we do not have the full state persisted.
		if err := s.rsAPI.QueryMembershipsForRoom(context.TODO(), membershipReq, membershipRes); err == nil {
			for _, ev := range membershipRes.JoinEvents {
				var membership gomatrixserverlib.MemberContent
				if err = json.Unmarshal(ev.Content, &membership); err != nil || ev.StateKey == nil {
					continue
				}
				log.Debugf("💜 %#v", membership)
			}
		} else {
			log.WithFields(log.Fields{
				"room_id": event.RoomID(),
			}).WithError(err).Errorf("Unable to get membership for room %s", event.RoomID())
		}
		// Fetches all `PushRules` from all `Users` in a `Room` by fetching all `Membership` `JOIN` events by `Room ID` that is inside the `Event`

	}
	return nil
}
