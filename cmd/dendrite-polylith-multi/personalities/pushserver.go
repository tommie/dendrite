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

package personalities

import (
	"github.com/matrix-org/dendrite/pushserver"
	roomserverAPI "github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/setup"
	"github.com/matrix-org/dendrite/setup/config"
)

func PushServer(base *setup.BaseDendrite, cfg *config.Dendrite, rsAPI roomserverAPI.RoomserverInternalAPI) {
	intAPI := pushserver.NewInternalAPI(base, rsAPI)

	pushserver.AddInternalRoutes(base.InternalAPIMux, intAPI)

	base.SetupAndServeHTTP(
		base.Cfg.PushServer.InternalAPI.Listen, // internal listener
		setup.NoListener,                       // external listener
		nil, nil,
	)
}