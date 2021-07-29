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
				Err: "No master key was found, either in the database or in the request!",
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
		// all of the key IDs specified in the signatures. If the key purpose is
		// NOT the master key then we also need to include the master key ID here
		// as we won't accept a self-signing key or a user-signing key without it.
		checkKeyIDs := make([]gomatrixserverlib.KeyID, 0, len(key.Signatures)+1)
		if purpose != gomatrixserverlib.CrossSigningKeyPurposeMaster {
			for keyID := range key.Signatures[req.UserID] {
				checkKeyIDs = append(checkKeyIDs, keyID)
			}
			if _, ok := key.Signatures[req.UserID][masterKeyID]; !ok {
				checkKeyIDs = append(checkKeyIDs, masterKeyID)
			}
		}

		// If there are no key IDs to check then there's no point marshalling
		// the JSON.
		if len(checkKeyIDs) == 0 && purpose == gomatrixserverlib.CrossSigningKeyPurposeMaster {
			continue
		}

		// Marshal the specific key back into JSON so that we can verify the
		// signature of it.
		keyJSON, err := json.Marshal(key)
		if err != nil {
			res.Error = &api.KeyError{
				Err:            fmt.Sprintf("The JSON of the key section is invalid: %s", err.Error()),
				IsMissingParam: true,
			}
			return
		}

		// Now verify the signatures.
		for _, keyID := range checkKeyIDs {
			if err := gomatrixserverlib.VerifyJSON(req.UserID, keyID, ed25519.PublicKey(masterKey), keyJSON); err != nil {
				res.Error = &api.KeyError{
					Err:                fmt.Sprintf("The signature verification failed using user %q key ID %q: %s", req.UserID, keyID, err.Error()),
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
