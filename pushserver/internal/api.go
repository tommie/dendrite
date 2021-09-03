package internal

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/matrix-org/dendrite/pushserver/api"
	"github.com/matrix-org/dendrite/pushserver/storage"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
	"github.com/sirupsen/logrus"
)

// PushserverInternalAPI implements api.PushserverInternalAPI
type PushserverInternalAPI struct {
	DB         storage.Database
	Cfg        *config.PushServer
	ServerName gomatrixserverlib.ServerName
}

func NewPushserverAPI(
	cfg *config.PushServer, pushserverDB storage.Database,
) *PushserverInternalAPI {
	a := &PushserverInternalAPI{
		DB:         pushserverDB,
		Cfg:        cfg,
		ServerName: cfg.Matrix.ServerName,
	}
	return a
}

func (a *PushserverInternalAPI) PerformPusherCreation(ctx context.Context, req *api.PerformPusherCreationRequest, res *api.PerformPusherCreationResponse) error {
	util.GetLogger(ctx).WithFields(logrus.Fields{
		"userId":       req.Device.UserID,
		"pushkey":      req.PushKey,
		"display_name": req.AppDisplayName,
	}).Info("PerformPusherCreation")
	local, _, err := gomatrixserverlib.SplitID('@', req.Device.UserID)
	if err != nil {
		return err
	}
	jsonData, err := json.Marshal(req.Data)
	if err != nil {
		return err
	}
	err = a.DB.CreatePusher(ctx, req.Device.SessionID, req.PushKey, req.Kind, req.AppID, req.AppDisplayName, req.DeviceDisplayName, req.ProfileTag, req.Language, string(jsonData), local)
	return err
}

func (a *PushserverInternalAPI) PerformPusherUpdate(ctx context.Context, req *api.PerformPusherUpdateRequest, res *api.PerformPusherUpdateResponse) error {
	util.GetLogger(ctx).WithFields(logrus.Fields{
		"localpart":    req.Device.UserID,
		"pushkey":      req.PushKey,
		"display_name": req.AppDisplayName,
	}).Info("PerformPusherUpdate")
	local, _, err := gomatrixserverlib.SplitID('@', req.Device.UserID)
	if err != nil {
		return err
	}
	jsonData, err := json.Marshal(req.Data)
	if err != nil {
		return err
	}
	err = a.DB.UpdatePusher(ctx, req.PushKey, req.Kind, req.AppID, req.AppDisplayName, req.DeviceDisplayName, req.ProfileTag, req.Language, string(jsonData), local)
	return err
}

func (a *PushserverInternalAPI) PerformPusherDeletion(ctx context.Context, req *api.PerformPusherDeletionRequest, res *api.PerformPusherDeletionResponse) error {
	util.GetLogger(ctx).WithField("user_id", req.UserID).WithField("pushkey", req.PushKey).WithField("app_id", req.AppID).Info("PerformPusherDeletion")
	local, domain, err := gomatrixserverlib.SplitID('@', req.UserID)
	if err != nil {
		return err
	}
	if domain != a.ServerName {
		return fmt.Errorf("cannot PerformPusherDeletion of remote users: got %s want %s", domain, a.ServerName)
	}
	err = a.DB.RemovePusher(ctx, req.AppID, req.PushKey, local)
	if err != nil {
		return err
	}
	return nil
}

func (a *PushserverInternalAPI) QueryPushers(ctx context.Context, req *api.QueryPushersRequest, res *api.QueryPushersResponse) error {
	local, domain, err := gomatrixserverlib.SplitID('@', req.UserID)
	if err != nil {
		return err
	}
	if domain != a.ServerName {
		return fmt.Errorf("cannot query pushers of remote users: got %s want %s", domain, a.ServerName)
	}
	pushers, err := a.DB.GetPushersByLocalpart(ctx, local)
	if err != nil {
		return err
	}
	res.Pushers = pushers
	return nil
}
