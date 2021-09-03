package api

import (
	"context"
	"encoding/json"

	"github.com/matrix-org/dendrite/userapi/api"
)

type PushserverInternalAPI interface {
	PerformPusherCreation(ctx context.Context, req *PerformPusherCreationRequest, res *PerformPusherCreationResponse) error
	PerformPusherDeletion(ctx context.Context, req *PerformPusherDeletionRequest, res *PerformPusherDeletionResponse) error
	PerformPusherUpdate(ctx context.Context, req *PerformPusherUpdateRequest, res *PerformPusherUpdateResponse) error
	QueryPushers(ctx context.Context, req *QueryPushersRequest, res *QueryPushersResponse) error
}

type PerformPusherDeletionRequest struct {
	AppID   string
	PushKey string
	UserID  string
}

type PerformPusherDeletionResponse struct {
}

// QueryPushersRequest is the request for QueryPushers
type QueryPushersRequest struct {
	UserID string
}

// QueryPushersResponse is the response for QueryPushers
type QueryPushersResponse struct {
	UserExists bool
	Pushers    []Pusher
}

// PerformPusherCreationRequest is the request for PerformPusherCreation
type PerformPusherCreationRequest struct {
	Device            *api.Device
	PushKey           string
	Kind              string
	AppID             string
	AppDisplayName    string
	DeviceDisplayName string
	ProfileTag        string
	Language          string
	Data              map[string]json.RawMessage
}

// PerformPusherCreationResponse is the response for PerformPusherCreation
type PerformPusherCreationResponse struct {
}

// PerformPusherUpdateRequest is the request for PerformPusherUpdate
type PerformPusherUpdateRequest struct {
	Device            *api.Device
	PushKey           string
	Kind              string
	AppID             string
	AppDisplayName    string
	DeviceDisplayName string
	ProfileTag        string
	Language          string
	Data              map[string]json.RawMessage
}

// PerformPusherUpdateResponse is the response for PerformPusherUpdate
type PerformPusherUpdateResponse struct {
}

// Pusher represents a push notification subscriber
type Pusher struct {
	SessionID         int64
	PushKey           string
	Kind              string
	AppID             string
	AppDisplayName    string
	DeviceDisplayName string
	ProfileTag        string
	Language          string
	Data              string
}
