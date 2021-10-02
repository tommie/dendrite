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

package auth

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/setup/config"
	uapi "github.com/matrix-org/dendrite/userapi/api"
)

func TestLoginFromJSONReader(t *testing.T) {
	ctx := context.Background()

	tsts := []struct {
		Name string
		Body string

		WantErrCode       string
		WantUsername      string
		WantDeviceID      string
		WantDeletedTokens []string
	}{
		{Name: "empty", WantErrCode: "M_BAD_JSON"},
		{
			Name: "passwordWorks",
			Body: `{
				"type": "m.login.password",
				"identifier": { "type": "m.id.user", "user": "alice" },
				"password": "herpassword",
				"device_id": "adevice"
            }`,
			WantUsername: "alice",
			WantDeviceID: "adevice",
		},
		{
			Name: "tokenWorks",
			Body: `{
				"type": "m.login.token",
				"token": "atoken",
				"device_id": "adevice"
            }`,
			WantUsername:      "@auser:example.com",
			WantDeviceID:      "adevice",
			WantDeletedTokens: []string{"atoken"},
		},
	}
	for _, tst := range tsts {
		t.Run(tst.Name, func(t *testing.T) {
			var accountDB fakeAccountDB
			var userAPI fakeUserInternalAPI
			cfg := &config.ClientAPI{
				Matrix: &config.Global{
					ServerName: serverName,
				},
			}
			login, cleanup, errRes := LoginFromJSONReader(ctx, strings.NewReader(tst.Body), &accountDB, &userAPI, cfg)
			if tst.WantErrCode == "" {
				if errRes != nil {
					t.Fatalf("LoginFromJSONReader failed: %+v", errRes)
				}
				cleanup(ctx, nil)
			} else {
				if errRes == nil {
					t.Fatalf("LoginFromJSONReader err: got %+v, want code %q", errRes, tst.WantErrCode)
				} else if merr, ok := errRes.JSON.(*jsonerror.MatrixError); ok && merr.ErrCode != tst.WantErrCode {
					t.Fatalf("LoginFromJSONReader err: got %+v, want code %q", errRes, tst.WantErrCode)
				}
				return
			}

			if login.Username() != tst.WantUsername {
				t.Errorf("Username: got %q, want %q", login.Username(), tst.WantUsername)
			}

			if login.DeviceID == nil {
				if tst.WantDeviceID != "" {
					t.Errorf("DeviceID: got %v, want %q", login.DeviceID, tst.WantDeviceID)
				}
			} else {
				if *login.DeviceID != tst.WantDeviceID {
					t.Errorf("DeviceID: got %q, want %q", *login.DeviceID, tst.WantDeviceID)
				}
			}

			if !reflect.DeepEqual(userAPI.DeletedTokens, tst.WantDeletedTokens) {
				t.Errorf("DeletedTokens: got %+v, want %+v", userAPI.DeletedTokens, tst.WantDeletedTokens)
			}
		})
	}
}

type fakeAccountDB struct {
	AccountDatabase
}

func (*fakeAccountDB) GetAccountByPassword(ctx context.Context, localpart, password string) (*uapi.Account, error) {
	return &uapi.Account{}, nil
}

type fakeUserInternalAPI struct {
	UserInternalAPIForLogin

	DeletedTokens []string
}

func (ua *fakeUserInternalAPI) PerformLoginTokenDeletion(ctx context.Context, req *uapi.PerformLoginTokenDeletionRequest, res *uapi.PerformLoginTokenDeletionResponse) error {
	ua.DeletedTokens = append(ua.DeletedTokens, req.Token)
	return nil
}

func (*fakeUserInternalAPI) QueryLoginToken(ctx context.Context, req *uapi.QueryLoginTokenRequest, res *uapi.QueryLoginTokenResponse) error {
	res.Data = &uapi.LoginTokenData{UserID: "@auser:example.com"}
	return nil
}
