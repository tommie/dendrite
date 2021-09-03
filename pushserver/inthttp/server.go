package inthttp

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/matrix-org/dendrite/internal/httputil"
	"github.com/matrix-org/dendrite/pushserver/api"
	"github.com/matrix-org/util"
)

// AddRoutes adds the RoomserverInternalAPI handlers to the http.ServeMux.
// nolint: gocyclo
func AddRoutes(r api.PushserverInternalAPI, internalAPIMux *mux.Router) {
	internalAPIMux.Handle(PerformPusherCreationPath,
		httputil.MakeInternalAPI("performPusherCreation", func(req *http.Request) util.JSONResponse {
			request := api.PerformPusherCreationRequest{}
			response := api.PerformPusherCreationResponse{}
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.MessageResponse(http.StatusBadRequest, err.Error())
			}
			if err := r.PerformPusherCreation(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
	internalAPIMux.Handle(PerformPusherUpdatePath,
		httputil.MakeInternalAPI("performPusherUpdate", func(req *http.Request) util.JSONResponse {
			request := api.PerformPusherUpdateRequest{}
			response := api.PerformPusherUpdateResponse{}
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.MessageResponse(http.StatusBadRequest, err.Error())
			}
			if err := r.PerformPusherUpdate(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
	internalAPIMux.Handle(PerformPusherDeletionPath,
		httputil.MakeInternalAPI("performPusherDeletion", func(req *http.Request) util.JSONResponse {
			request := api.PerformPusherDeletionRequest{}
			response := api.PerformPusherDeletionResponse{}
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.MessageResponse(http.StatusBadRequest, err.Error())
			}
			if err := r.PerformPusherDeletion(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
	internalAPIMux.Handle(QueryPushersPath,
		httputil.MakeInternalAPI("queryPushers", func(req *http.Request) util.JSONResponse {
			request := api.QueryPushersRequest{}
			response := api.QueryPushersResponse{}
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.MessageResponse(http.StatusBadRequest, err.Error())
			}
			if err := r.QueryPushers(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
}
