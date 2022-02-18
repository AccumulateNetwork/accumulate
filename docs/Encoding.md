# Binary Encoding

Each field is prefixed with a field number from 1 to 32, followed by the value.
Array-valued fields are encoded by iterating over the array, encoding each
value, and prefixing each value with the field number.

## Values

+ `hash` is a 32-byte hash, aka `chain`
  - Encoded without modification, as 32 bytes.
+ `int` is a signed integer, aka `varint`
  - Encoded with signed varint encoding.
+ `uint` is an unsigned integer, aka `uvarint`
  - Encoded with unsigned varint encoding.
+ `bool` is a boolean
  - Encoded as 0 or 1, as an unsigned integer.
+ `time` is a date and time
  - Encoded as a Unix timestamp in UTC as an signed varint.
+ `bytes` is an array of bytes
  - Length is encoded as an unsigned varint;
  - Followed by the bytes, without modification.
+ `string` is a string
  - Encoded as a byte array.
+ `duration` is an elapsed time or duration
  - Encoded as two unsigned varints, seconds and nanoseconds.
+ `url` is an Accumulate URL
  - Encoded as a string.
+ `bigint` is an arbitrary precision integer
  - Encoded as a big-endian byte array.

Values that implement the binary marshaller interface are marshalled to a byte
array and then encoded as a byte array (prefixed with the length).

## Transactions and Accounts

Transanctions and accounts have an implicit first field that denotes the
transaction or account type, encoded as an unsigned varint.

## Example

1. Send tokens transaction with two recipients

| Field               | Value                  | Encoded
| ------------------- | ---------------------- | -------
| Header (1)          | *Header*               | `01 1B 01126163 633A2F2F 616C6963 652F746F 6B656E73 02030301 04C803`
| Body (2)            | *Send tokens*          | `02 34 01030415 01106163 633A2F2F 626F622F 746F6B65 6E730201 0A041901 14616363 3A2F2F63 6861726C 69652F74 6F6B656E 73020119`
| **Header**          | <hr />                 | <hr />
| Origin (1)          | `acc://alice/tokens`   | `01 12 6163633A 2F2F616C 6963652F 746F6B65 6E73`
| Key page height (2) | `3`                    | `02 03`
| Key page height (3) | `1`                    | `03 01`
| Nonce (4)           | `456`                  | `04 C803`
| **Send tokens**     | <hr />                 | <hr />
| Type (1)            | `SendTokens = 3`       | `01 03`
| To (4)              | *Recipient 1*          | `04 15 01106163 633A2F2F 626F622F 746F6B65 6E730201 0A`
| To (4)              | *Recipient 2*          | `04 19 01146163 633A2F2F 63686172 6C69652F 746F6B65 6E730201 19`
| **Recipient 1**     | <hr />                 | <hr />
| Origin (1)          | `acc://bob/tokens`     | `01 10 6163633A 2F2F626F 622F746F 6B656E73`
| Amount (2)          | `10`                   | `02 01 0A`
| **Recipient 2**     | <hr />                 | <hr />
| Origin (1)          | `acc://charlie/tokens` | `01 14 6163633A 2F2F6368 61726C69 652F746F 6B656E73`
| Amount (2)          | `25`                   | `02 01 19`
