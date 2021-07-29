package internal

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
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
	hasMasterKey := false

	if len(req.MasterKey.Keys) > 0 {
		if err := sanityCheckKey(req.MasterKey, req.UserID, gomatrixserverlib.CrossSigningKeyPurposeMaster); err != nil {
			res.Error = &api.KeyError{
				Err: "Master key sanity check failed: " + err.Error(),
			}
			return
		}
		hasMasterKey = true
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

	// If the user hasn't given a new master key, then let's go and get their
	// existing keys from the database.
	var masterKey gomatrixserverlib.Base64Bytes
	if !hasMasterKey {
		existingKeys, err := a.DB.CrossSigningKeysForUser(ctx, req.UserID)
		if err != nil {
			res.Error = &api.KeyError{
				Err: "User-signing key sanity check failed: " + err.Error(),
			}
			return
		}

		masterKey, hasMasterKey = existingKeys[gomatrixserverlib.CrossSigningKeyPurposeMaster]
		if !hasMasterKey {
			res.Error = &api.KeyError{
				Err:            "No master key was found, either in the database or in the request!",
				IsMissingParam: true,
			}
			return
		}
	} else {
		for _, keyData := range req.MasterKey.Keys { // iterates once, see sanityCheckKey
			masterKey = keyData
		}
	}
	masterKeyID := gomatrixserverlib.KeyID(fmt.Sprintf("ed25519:%s", masterKey.Encode()))

	// Work out which things we need to verify the signatures for.
	toVerify := make(map[gomatrixserverlib.CrossSigningKeyPurpose]gomatrixserverlib.CrossSigningKey, 3)
	toStore := api.CrossSigningKeyMap{}
	if len(req.MasterKey.Keys) > 0 {
		toVerify[gomatrixserverlib.CrossSigningKeyPurposeMaster] = req.MasterKey
	}
	if len(req.SelfSigningKey.Keys) > 0 {
		toVerify[gomatrixserverlib.CrossSigningKeyPurposeSelfSigning] = req.SelfSigningKey
	}
	if len(req.SelfSigningKey.Keys) > 0 {
		toVerify[gomatrixserverlib.CrossSigningKeyPurposeUserSigning] = req.UserSigningKey
	}
	for purpose, key := range toVerify {
		// Collect together the key IDs we need to verify with. This will include
		// all of the key IDs specified in the signatures. We don't do this for
		// the master key because we have no means to verify the signatures - we
		// instead just need to store them.
		if purpose != gomatrixserverlib.CrossSigningKeyPurposeMaster {
			// Marshal the specific key back into JSON so that we can verify the
			// signature of it.
			keyJSON, err := json.Marshal(key)
			if err != nil {
				res.Error = &api.KeyError{
					Err: fmt.Sprintf("The JSON of the key section is invalid: %s", err.Error()),
				}
				return
			}

			// Now check if the subkey is signed by the master key.
			if err := gomatrixserverlib.VerifyJSON(req.UserID, masterKeyID, ed25519.PublicKey(masterKey), keyJSON); err != nil {
				res.Error = &api.KeyError{
					Err:                fmt.Sprintf("The %q sub-key failed master key signature verification: %s", purpose, err.Error()),
					IsInvalidSignature: true,
				}
				return
			}
		}

		// If we've reached this point then all the signatures are valid so
		// add the key to the list of keys to store.
		for _, keyData := range key.Keys { // iterates once, see sanityCheckKey
			toStore[purpose] = keyData
		}
	}

	if err := a.DB.StoreCrossSigningKeysForUser(ctx, req.UserID, toStore, req.StreamID); err != nil {
		res.Error = &api.KeyError{
			Err: fmt.Sprintf("a.DB.StoreCrossSigningKeysForUser: %s", err),
		}
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
			keyID := gomatrixserverlib.KeyID("ed25519:" + b64)
			key := gomatrixserverlib.CrossSigningKey{
				UserID: userID,
				Usage: []gomatrixserverlib.CrossSigningKeyPurpose{
					keyType,
				},
				Keys: map[gomatrixserverlib.KeyID]gomatrixserverlib.Base64Bytes{
					keyID: keyData,
				},
			}

			sigs, err := a.DB.CrossSigningSigsForTarget(ctx, userID, keyID)
			if err != nil {
				logrus.WithError(err).Errorf("Failed to get cross-signing signatures for user %q key %q", userID, keyID)
				return fmt.Errorf("a.DB.CrossSigningSigsForTarget (%q key %q): %w", userID, keyID, err)
			}

			appendSignature := func(originUserID string, originKeyID gomatrixserverlib.KeyID, signature gomatrixserverlib.Base64Bytes) {
				if key.Signatures == nil {
					key.Signatures = api.CrossSigningSigMap{}
				}
				if _, ok := key.Signatures[originUserID]; !ok {
					key.Signatures[originUserID] = make(map[gomatrixserverlib.KeyID]gomatrixserverlib.Base64Bytes)
				}
				key.Signatures[originUserID][originKeyID] = signature
			}

			for originUserID, forOrigin := range sigs {
				for originKeyID, signature := range forOrigin {
					switch {
					case req.UserID != "" && originUserID == req.UserID:
						appendSignature(originUserID, originKeyID, signature)
					case originUserID == userID:
						appendSignature(originUserID, originKeyID, signature)
					}
				}
			}

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
