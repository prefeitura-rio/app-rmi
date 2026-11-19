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

// BSONStringish decodes identifiers written as BSON string, integer or double into a string
// (BigQuery INTEGER fields such as id_chamado / id_andamento often land as numbers in Mongo).
type BSONStringish string

// UnmarshalBSONValue implements bson.ValueUnmarshaler.
func (s *BSONStringish) UnmarshalBSONValue(t bsontype.Type, data []byte) error {
	raw := bson.RawValue{Type: t, Value: data}
	switch t {
	case bson.TypeNull, bson.TypeUndefined:
		*s = ""
	case bson.TypeString:
		*s = BSONStringish(raw.StringValue())
	case bson.TypeInt32:
		*s = BSONStringish(strconv.FormatInt(int64(raw.Int32()), 10))
	case bson.TypeInt64:
		*s = BSONStringish(strconv.FormatInt(raw.Int64(), 10))
	case bson.TypeDouble:
		*s = BSONStringish(strconv.FormatInt(int64(raw.Double()), 10))
	default:
		return fmt.Errorf("unsupported BSON type %s for stringish id", t)
	}
	return nil
}

// String returns the decoded value.
func (s BSONStringish) String() string { return string(s) }

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
