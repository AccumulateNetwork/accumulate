package errors

// GENERATED BY go run ./tools/cmd/gen-enum. DO NOT EDIT.

import (
	"encoding/json"
	"fmt"
	"strings"
)

// OK means the request completed successfully.
const OK Status = 200

// Delivered means the transaction has been delivered.
const Delivered Status = 201

// Pending means the transaction is pending.
const Pending Status = 202

// Remote means the transaction is a local reference to a remote.
const Remote Status = 203

// WrongPartition means the requested resource is assigned to a different network partition.
const WrongPartition Status = 301

// BadRequest means the request was invalid.
const BadRequest Status = 400

// Unauthenticated means the signature could not be validated.
const Unauthenticated Status = 401

// InsufficientCredits means the signer does not have sufficient credits to execute the transaction.
const InsufficientCredits Status = 402

// Unauthorized means the signer is not authorized to sign the transaction.
const Unauthorized Status = 403

// NotFound means a record could not be found.
const NotFound Status = 404

// NotAllowed means the requested action could not be performed.
const NotAllowed Status = 405

// Conflict means the request failed due to a conflict.
const Conflict Status = 409

// BadSignerVersion means the signer version does not match.
const BadSignerVersion Status = 411

// BadTimestamp means the timestamp is invalid.
const BadTimestamp Status = 412

// BadUrlLength means the url length is too big.
const BadUrlLength Status = 413

// Internal means an internal error occurred.
const Internal Status = 500

// Unknown means an unknown error occurred.
const Unknown Status = 501

// Encoding means encoding or decoding failed.
const Encoding Status = 502

// Fatal means something has gone seriously wrong.
const Fatal Status = 503

// GetEnumValue returns the value of the Status
func (v Status) GetEnumValue() uint64 { return uint64(v) }

// SetEnumValue sets the value. SetEnumValue returns false if the value is invalid.
func (v *Status) SetEnumValue(id uint64) bool {
	u := Status(id)
	switch u {
	case OK, Delivered, Pending, Remote, WrongPartition, BadRequest, Unauthenticated, InsufficientCredits, Unauthorized, NotFound, NotAllowed, Conflict, BadSignerVersion, BadTimestamp, BadUrlLength, Internal, Unknown, Encoding, Fatal:
		*v = u
		return true
	default:
		return false
	}
}

// String returns the name of the Status.
func (v Status) String() string {
	switch v {
	case OK:
		return "ok"
	case Delivered:
		return "delivered"
	case Pending:
		return "pending"
	case Remote:
		return "remote"
	case WrongPartition:
		return "wrongPartition"
	case BadRequest:
		return "badRequest"
	case Unauthenticated:
		return "unauthenticated"
	case InsufficientCredits:
		return "insufficientCredits"
	case Unauthorized:
		return "unauthorized"
	case NotFound:
		return "notFound"
	case NotAllowed:
		return "notAllowed"
	case Conflict:
		return "conflict"
	case BadSignerVersion:
		return "badSignerVersion"
	case BadTimestamp:
		return "badTimestamp"
	case BadUrlLength:
		return "badUrlLength"
	case Internal:
		return "internal"
	case Unknown:
		return "unknown"
	case Encoding:
		return "encoding"
	case Fatal:
		return "fatal"
	default:
		return fmt.Sprintf("Status:%d", v)
	}
}

// StatusByName returns the named Status.
func StatusByName(name string) (Status, bool) {
	switch strings.ToLower(name) {
	case "ok":
		return OK, true
	case "delivered":
		return Delivered, true
	case "pending":
		return Pending, true
	case "remote":
		return Remote, true
	case "wrongpartition":
		return WrongPartition, true
	case "badrequest":
		return BadRequest, true
	case "unauthenticated":
		return Unauthenticated, true
	case "insufficientcredits":
		return InsufficientCredits, true
	case "unauthorized":
		return Unauthorized, true
	case "notfound":
		return NotFound, true
	case "notallowed":
		return NotAllowed, true
	case "conflict":
		return Conflict, true
	case "badsignerversion":
		return BadSignerVersion, true
	case "badtimestamp":
		return BadTimestamp, true
	case "badurllength":
		return BadUrlLength, true
	case "internal":
		return Internal, true
	case "unknown":
		return Unknown, true
	case "encoding":
		return Encoding, true
	case "fatal":
		return Fatal, true
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
