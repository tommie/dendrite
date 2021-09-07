package inthttp

import (
	"context"
	"errors"
	"net/http"

	"github.com/matrix-org/dendrite/internal/httputil"
	"github.com/matrix-org/dendrite/pushserver/api"
	"github.com/opentracing/opentracing-go"
)

type httpPushserverInternalAPI struct {
	roomserverURL string
	httpClient    *http.Client
}

const (
	PerformPusherSetPath      = "/userapi/performPusherSet"
	PerformPusherDeletionPath = "/userapi/performPusherDeletion"
	QueryPushersPath          = "/userapi/queryPushers"
)

// NewRoomserverClient creates a PushserverInternalAPI implemented by talking to a HTTP POST API.
// If httpClient is nil an error is returned
func NewPushserverClient(
	pushserverURL string,
	httpClient *http.Client,
) (api.PushserverInternalAPI, error) {
	if httpClient == nil {
		return nil, errors.New("NewPushserverClient: httpClient is <nil>")
	}
	return &httpPushserverInternalAPI{
		roomserverURL: pushserverURL,
		httpClient:    httpClient,
	}, nil
}

func (h *httpPushserverInternalAPI) PerformPusherSet(
	ctx context.Context,
	request *api.PerformPusherSetRequest,
	response struct{},
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "PerformPusherSet")
	defer span.Finish()

	apiURL := h.roomserverURL + PerformPusherSetPath
	return httputil.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}

func (h *httpPushserverInternalAPI) PerformPusherDeletion(ctx context.Context, req *api.PerformPusherDeletionRequest, res struct{}) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "PerformPusherDeletion")
	defer span.Finish()

	apiURL := h.roomserverURL + PerformPusherSetPath
	return httputil.PostJSON(ctx, span, h.httpClient, apiURL, req, res)
}

func (h *httpPushserverInternalAPI) QueryPushers(ctx context.Context, req *api.QueryPushersRequest, res *api.QueryPushersResponse) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "QueryPushers")
	defer span.Finish()

	apiURL := h.roomserverURL + QueryPushersPath
	return httputil.PostJSON(ctx, span, h.httpClient, apiURL, req, res)
}
