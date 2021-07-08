package helpers

import (
	"context"
	"fmt"
	"sync"

	"github.com/matrix-org/dendrite/roomserver/storage"
	"github.com/matrix-org/dendrite/roomserver/types"
)

type CachedDB struct {
	mutex       sync.RWMutex
	eventsByID  map[string]*types.Event
	eventsByNID map[types.EventNID]*types.Event
	storage.Database
}

func NewCachedDB(db storage.Database) *CachedDB {
	return &CachedDB{
		Database:    db,
		eventsByID:  make(map[string]*types.Event),
		eventsByNID: make(map[types.EventNID]*types.Event),
	}
}

func (c *CachedDB) Events(ctx context.Context, eventNIDs []types.EventNID) ([]types.Event, error) {
	fmt.Println("Want", eventNIDs)
	events := make([]types.Event, len(eventNIDs))
	retrieve := make([]types.EventNID, 0, len(eventNIDs))
	c.mutex.RLock()
	for i, eventNID := range eventNIDs {
		if cached, ok := c.eventsByNID[eventNID]; ok {
			events[i] = *cached
			fmt.Println(i, "Existing", cached, cached.EventID(), cached.EventNID, cached.Type(), *cached.StateKey())
		} else {
			retrieve = append(retrieve, eventNID)
		}
	}
	c.mutex.RUnlock()
	var retrieved []types.Event
	var err error
	if len(retrieve) > 0 {
		retrieved, err = c.Database.Events(ctx, retrieve)
		if err != nil {
			return nil, err
		}
	}
	c.mutex.Lock()
	defer c.mutex.Unlock()
	for i, event := range retrieved {
		c.eventsByID[event.EventID()] = &retrieved[i]
		c.eventsByNID[event.EventNID] = &retrieved[i]
	}
	for i, eventNID := range eventNIDs {
		if cached, ok := c.eventsByNID[eventNID]; ok {
			fmt.Println(i, "Found", cached, cached.EventID(), cached.EventNID, cached.Type(), *cached.StateKey(), string(cached.Content()))
			events[i] = *cached
		}
	}
	fmt.Println("Returning", events)
	return events, nil
}
