package internal

import (
	"context"

	"github.com/matrix-org/dendrite/pushserver/api"
	"github.com/matrix-org/dendrite/pushserver/storage"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/util"
	"github.com/sirupsen/logrus"
)

// PushserverInternalAPI implements api.PushserverInternalAPI
type PushserverInternalAPI struct {
	DB  storage.Database
	Cfg *config.PushServer
}

func NewPushserverAPI(
	cfg *config.PushServer, pushserverDB storage.Database,
) *PushserverInternalAPI {
	a := &PushserverInternalAPI{
		DB:  pushserverDB,
		Cfg: cfg,
	}
	return a
}

func (a *PushserverInternalAPI) PerformPusherSet(ctx context.Context, req *api.PerformPusherSetRequest, res struct{}) error {
	util.GetLogger(ctx).WithFields(logrus.Fields{
		"localpart":    req.Localpart,
		"pushkey":      req.PushKey,
		"display_name": req.AppDisplayName,
	}).Info("PerformPusherCreation")
	if !req.Append {
		err := a.DB.RemovePushers(ctx, req.AppID, req.AppDisplayName)
		if err != nil {
			return err
		}
	} else {
		if req.Kind == "" {
			return a.DB.RemovePusher(ctx, req.AppID, req.AppDisplayName, req.Localpart)
		}
	}
	return a.DB.CreatePusher(ctx, req.Pusher, req.Localpart)
}

func (a *PushserverInternalAPI) PerformPusherDeletion(ctx context.Context, req *api.PerformPusherDeletionRequest, res struct{}) error {
	pushers, err := a.DB.GetPushers(ctx, req.Localpart)
	if err != nil {
		return err
	}
	for i := range pushers {
		logrus.Warnf("pusher session: %d, req session: %d", pushers[i].SessionID, req.SessionID)
		if pushers[i].SessionID != req.SessionID {
			err := a.DB.RemovePusher(ctx, pushers[i].AppID, pushers[i].PushKey, req.Localpart)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (a *PushserverInternalAPI) QueryPushers(ctx context.Context, req *api.QueryPushersRequest, res *api.QueryPushersResponse) error {
	var err error
	res.Pushers, err = a.DB.GetPushers(ctx, req.Localpart)
	return err
}

// TODO: THIS IS AN EXAMPLE QueryExample implements PushserverAliasAPI
func (p *PushserverInternalAPI) QueryExample(
	ctx context.Context,
	request *api.QueryExampleRequest,
	response *api.QueryExampleResponse,
) error {
	// Implement QueryExample here!

	return nil
}

