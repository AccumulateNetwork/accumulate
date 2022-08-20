package abcicodes

// OK indicates the request succeeded.
const OK = 0

// EncodingError indicates something could not be decoded or encoded.
const EncodingError = 1

// Failed indicates the request failed.
const Failed = 2

// DidPanic indicates the request failed due to a fatal error.
const DidPanic = 3

// UnknownError indicates the request failed due to an unknown error.
const UnknownError = 4
