package internal

import (
	"context"
	"fmt"
	"strings"

	"github.com/matrix-org/dendrite/keyserver/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/sirupsen/logrus"
)

func sanityCheckKey(key gomatrixserverlib.CrossSigningKey, userID string, purpose gomatrixserverlib.CrossSigningKeyPurpose) error {
	// Is there exactly one key?
	if len(key.Keys) != 1 {
		return fmt.Errorf("should contain exactly one key")
	}

	// Does the key ID match the key value? Iterates exactly once
	for keyID, keyData := range key.Keys {
		b64 := keyData.Encode()
		tokens := strings.Split(string(keyID), ":")
		if len(tokens) != 2 {
			return fmt.Errorf("key ID is incorrectly formatted")
		}
		if tokens[1] != b64 {
			return fmt.Errorf("key ID isn't correct")
		}
	}

	// Does the key claim to be from the right user?
	if userID != key.UserID {
		return fmt.Errorf("key has a user ID mismatch")
	}

	// Does the key contain the correct purpose?
	useful := false
	for _, usage := range key.Usage {
		if usage == purpose {
			useful = true
		}
	}
	if !useful {
		return fmt.Errorf("key does not contain correct usage purpose")
	}

	return nil
}

func (a *KeyInternalAPI) PerformUploadDeviceKeys(ctx context.Context, req *api.PerformUploadDeviceKeysRequest, res *api.PerformUploadDeviceKeysResponse) {
	if len(req.MasterKey.Keys) > 0 {
		if err := sanityCheckKey(req.MasterKey, req.UserID, gomatrixserverlib.CrossSigningKeyPurposeMaster); err != nil {
			res.Error = &api.KeyError{
				Err: "Master key sanity check failed: " + err.Error(),
			}
			return
		}
	}

	if len(req.SelfSigningKey.Keys) > 0 {
		if err := sanityCheckKey(req.SelfSigningKey, req.UserID, gomatrixserverlib.CrossSigningKeyPurposeSelfSigning); err != nil {
			res.Error = &api.KeyError{
				Err: "Self-signing key sanity check failed: " + err.Error(),
			}
			return
		}
	}

	if len(req.UserSigningKey.Keys) > 0 {
		if err := sanityCheckKey(req.UserSigningKey, req.UserID, gomatrixserverlib.CrossSigningKeyPurposeUserSigning); err != nil {
			res.Error = &api.KeyError{
				Err: "User-signing key sanity check failed: " + err.Error(),
			}
			return
		}
	}

	res.Error = &api.KeyError{
		Err: "Not supported yet",
	}
}

func (a *KeyInternalAPI) PerformUploadDeviceSignatures(ctx context.Context, req *api.PerformUploadDeviceSignaturesRequest, res *api.PerformUploadDeviceSignaturesResponse) {
	res.Error = &api.KeyError{
		Err: "Not supported yet",
	}
}

func (a *KeyInternalAPI) crossSigningKeys(
	ctx context.Context, req *api.QueryKeysRequest, res *api.QueryKeysResponse,
) error {
	for userID := range req.UserToDevices {
		keys, err := a.DB.CrossSigningKeysForUser(ctx, userID)
		if err != nil {
			logrus.WithError(err).Errorf("Failed to get cross-signing keys for user %q", userID)
			return fmt.Errorf("a.DB.CrossSigningKeysForUser (%q): %w", userID, err)
		}

		for keyType, keyData := range keys {
			b64 := keyData.Encode()
			key := gomatrixserverlib.CrossSigningKey{
				UserID: userID,
				Usage: []gomatrixserverlib.CrossSigningKeyPurpose{
					keyType,
				},
				Keys: map[gomatrixserverlib.KeyID]gomatrixserverlib.Base64Bytes{
					gomatrixserverlib.KeyID("ed25519:" + b64): keyData,
				},
			}

			// TODO: populate signatures

			switch keyType {
			case gomatrixserverlib.CrossSigningKeyPurposeMaster:
				res.MasterKeys[userID] = key

			case gomatrixserverlib.CrossSigningKeyPurposeSelfSigning:
				res.SelfSigningKeys[userID] = key

			case gomatrixserverlib.CrossSigningKeyPurposeUserSigning:
				res.UserSigningKeys[userID] = key
			}
		}
	}

	return nil
}
