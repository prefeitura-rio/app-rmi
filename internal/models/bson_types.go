package models

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/bsontype"
)

// BSONTimeString decodes a timestamp written either as a BSON string or as a BSON datetime
// (the shape depends on the loader) into a string; datetimes become RFC3339 in UTC.
type BSONTimeString string

// UnmarshalBSONValue implements bson.ValueUnmarshaler.
func (s *BSONTimeString) UnmarshalBSONValue(t bsontype.Type, data []byte) error {
	raw := bson.RawValue{Type: t, Value: data}
	switch t {
	case bson.TypeNull, bson.TypeUndefined:
		*s = ""
	case bson.TypeString:
		*s = BSONTimeString(raw.StringValue())
	case bson.TypeDateTime:
		*s = BSONTimeString(raw.Time().UTC().Format(time.RFC3339))
	default:
		return fmt.Errorf("unsupported BSON type %s for timestamp", t)
	}
	return nil
}

// BSONFlag decodes a 0/1 flag written as a BSON integer, double, boolean or string.
// Anything other than 1 (or true) is unset, so an unexpected value never marks an entry public.
type BSONFlag bool

// IsSet reports whether the flag equals 1.
func (f BSONFlag) IsSet() bool { return bool(f) }

// UnmarshalBSONValue implements bson.ValueUnmarshaler.
func (f *BSONFlag) UnmarshalBSONValue(t bsontype.Type, data []byte) error {
	raw := bson.RawValue{Type: t, Value: data}
	switch t {
	case bson.TypeNull, bson.TypeUndefined:
		*f = false
	case bson.TypeInt32:
		*f = raw.Int32() == 1
	case bson.TypeInt64:
		*f = raw.Int64() == 1
	case bson.TypeDouble:
		*f = raw.Double() == 1
	case bson.TypeBoolean:
		*f = BSONFlag(raw.Boolean())
	case bson.TypeString:
		n, err := strconv.Atoi(strings.TrimSpace(raw.StringValue()))
		*f = err == nil && n == 1
	default:
		return fmt.Errorf("unsupported BSON type %s for flag", t)
	}
	return nil
}
