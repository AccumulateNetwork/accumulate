package errors

// GENERATED BY go run ./tools/cmd/gen-enum. DO NOT EDIT.

import (
	"encoding/json"
	"fmt"
	"strings"
)

// StatusOK means the request completed successfully.
const StatusOK Status = 200

// StatusDelivered means the transaction has been delivered.
const StatusDelivered Status = 201

// StatusPending means the transaction is pending.
const StatusPending Status = 202

// StatusRemote means the transaction is a local reference to a remote.
const StatusRemote Status = 203

// StatusWrongPartition means the requested resource is assigned to a different network partition.
const StatusWrongPartition Status = 301

// StatusBadRequest means the request was invalid.
const StatusBadRequest Status = 400

// StatusUnauthenticated means the signature could not be validated.
const StatusUnauthenticated Status = 401

// StatusInsufficientCredits means the signer does not have sufficient credits to execute the transaction.
const StatusInsufficientCredits Status = 402

// StatusUnauthorized means the signer is not authorized to sign the transaction.
const StatusUnauthorized Status = 403

// StatusNotFound means a record could not be found.
const StatusNotFound Status = 404

// StatusConflict means the request failed due to a conflict.
const StatusConflict Status = 409

// StatusBadSignerVersion means the signer version does not match.
const StatusBadSignerVersion Status = 411

// StatusBadTimestamp means the timestamp is invalid.
const StatusBadTimestamp Status = 412

// StatusInternalError means an internal error occured.
const StatusInternalError Status = 500

// StatusUnknown means an unknown error occured.
const StatusUnknown Status = 501

// StatusEncodingError means encoding or decoding failed.
const StatusEncodingError Status = 502

// GetEnumValue returns the value of the Status
func (v Status) GetEnumValue() uint64 { return uint64(v) }

// SetEnumValue sets the value. SetEnumValue returns false if the value is invalid.
func (v *Status) SetEnumValue(id uint64) bool {
	u := Status(id)
	switch u {
	case StatusOK, StatusDelivered, StatusPending, StatusRemote, StatusWrongPartition, StatusBadRequest, StatusUnauthenticated, StatusInsufficientCredits, StatusUnauthorized, StatusNotFound, StatusConflict, StatusBadSignerVersion, StatusBadTimestamp, StatusInternalError, StatusUnknown, StatusEncodingError:
		*v = u
		return true
	default:
		return false
	}
}

// String returns the name of the Status.
func (v Status) String() string {
	switch v {
	case StatusOK:
		return "ok"
	case StatusDelivered:
		return "delivered"
	case StatusPending:
		return "pending"
	case StatusRemote:
		return "remote"
	case StatusWrongPartition:
		return "wrongPartition"
	case StatusBadRequest:
		return "badRequest"
	case StatusUnauthenticated:
		return "unauthenticated"
	case StatusInsufficientCredits:
		return "insufficientCredits"
	case StatusUnauthorized:
		return "unauthorized"
	case StatusNotFound:
		return "notFound"
	case StatusConflict:
		return "conflict"
	case StatusBadSignerVersion:
		return "badSignerVersion"
	case StatusBadTimestamp:
		return "badTimestamp"
	case StatusInternalError:
		return "internalError"
	case StatusUnknown:
		return "unknown"
	case StatusEncodingError:
		return "encodingError"
	default:
		return fmt.Sprintf("Status:%d", v)
	}
}

// StatusByName returns the named Status.
func StatusByName(name string) (Status, bool) {
	switch strings.ToLower(name) {
	case "ok":
		return StatusOK, true
	case "delivered":
		return StatusDelivered, true
	case "pending":
		return StatusPending, true
	case "remote":
		return StatusRemote, true
	case "wrongpartition":
		return StatusWrongPartition, true
	case "badrequest":
		return StatusBadRequest, true
	case "unauthenticated":
		return StatusUnauthenticated, true
	case "insufficientcredits":
		return StatusInsufficientCredits, true
	case "unauthorized":
		return StatusUnauthorized, true
	case "notfound":
		return StatusNotFound, true
	case "conflict":
		return StatusConflict, true
	case "badsignerversion":
		return StatusBadSignerVersion, true
	case "badtimestamp":
		return StatusBadTimestamp, true
	case "internalerror":
		return StatusInternalError, true
	case "unknown":
		return StatusUnknown, true
	case "encodingerror":
		return StatusEncodingError, true
	default:
		return 0, false
	}
}

// MarshalJSON marshals the Status to JSON as a string.
func (v Status) MarshalJSON() ([]byte, error) {
	return json.Marshal(v.String())
}

// UnmarshalJSON unmarshals the Status from JSON as a string.
func (v *Status) UnmarshalJSON(data []byte) error {
	var s string
	err := json.Unmarshal(data, &s)
	if err != nil {
		return err
	}

	var ok bool
	*v, ok = StatusByName(s)
	if !ok || strings.ContainsRune(v.String(), ':') {
		return fmt.Errorf("invalid Status %q", s)
	}
	return nil
}
