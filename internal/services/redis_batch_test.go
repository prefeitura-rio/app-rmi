package services

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestNewRedisBatch(t *testing.T) {
	logger := zap.NewNop()

	batch := NewRedisBatch(logger)

	require.NotNil(t, batch)
	assert.NotNil(t, batch.operations)
	assert.NotNil(t, batch.keys)
	assert.NotNil(t, batch.logger)
	assert.Equal(t, 0, batch.Size())
}

func TestRedisBatch_AddGet(t *testing.T) {
	logger := zap.NewNop()
	batch := NewRedisBatch(logger)

	batch.AddGet("test:key1")
	batch.AddGet("test:key2")
	batch.AddGet("test:key3")

	assert.Equal(t, 3, batch.Size())
	assert.Len(t, batch.keys, 3)
	assert.Contains(t, batch.keys, "test:key1")
	assert.Contains(t, batch.keys, "test:key2")
	assert.Contains(t, batch.keys, "test:key3")
}

func TestRedisBatch_AddSet(t *testing.T) {
	logger := zap.NewNop()
	batch := NewRedisBatch(logger)

	batch.AddSet("test:key1", "value1", 1*time.Hour)
	batch.AddSet("test:key2", "value2", 2*time.Hour)

	assert.Equal(t, 2, batch.Size())
	assert.Len(t, batch.keys, 2)
	assert.Contains(t, batch.keys, "test:key1")
	assert.Contains(t, batch.keys, "test:key2")
}

func TestRedisBatch_AddDel(t *testing.T) {
	logger := zap.NewNop()
	batch := NewRedisBatch(logger)

	batch.AddDel("test:key1", "test:key2", "test:key3")

	assert.Equal(t, 1, batch.Size()) // One operation
	assert.Len(t, batch.keys, 3)     // Three keys
	assert.Contains(t, batch.keys, "test:key1")
	assert.Contains(t, batch.keys, "test:key2")
	assert.Contains(t, batch.keys, "test:key3")
}

func TestRedisBatch_AddDel_SingleKey(t *testing.T) {
	logger := zap.NewNop()
	batch := NewRedisBatch(logger)

	batch.AddDel("test:single")

	assert.Equal(t, 1, batch.Size())
	assert.Len(t, batch.keys, 1)
	assert.Contains(t, batch.keys, "test:single")
}

func TestRedisBatch_MixedOperations(t *testing.T) {
	logger := zap.NewNop()
	batch := NewRedisBatch(logger)

	batch.AddGet("test:get1")
	batch.AddSet("test:set1", "value1", 1*time.Hour)
	batch.AddGet("test:get2")
	batch.AddDel("test:del1", "test:del2")
	batch.AddSet("test:set2", "value2", 2*time.Hour)

	assert.Equal(t, 5, batch.Size())
	assert.Len(t, batch.keys, 6) // get1, set1, get2, del1, del2, set2
}

func TestRedisBatch_Size(t *testing.T) {
	logger := zap.NewNop()
	batch := NewRedisBatch(logger)

	assert.Equal(t, 0, batch.Size())

	batch.AddGet("key1")
	assert.Equal(t, 1, batch.Size())

	batch.AddSet("key2", "value", time.Hour)
	assert.Equal(t, 2, batch.Size())

	batch.AddDel("key3", "key4")
	assert.Equal(t, 3, batch.Size())
}

func TestRedisBatch_Clear(t *testing.T) {
	logger := zap.NewNop()
	batch := NewRedisBatch(logger)

	// Add operations
	batch.AddGet("key1")
	batch.AddSet("key2", "value", time.Hour)
	batch.AddDel("key3")

	assert.Equal(t, 3, batch.Size())

	// Clear
	batch.Clear()

	assert.Equal(t, 0, batch.Size())
	assert.Len(t, batch.operations, 0)
	assert.Len(t, batch.keys, 0)
}

func TestRedisBatch_ClearAndReuse(t *testing.T) {
	logger := zap.NewNop()
	batch := NewRedisBatch(logger)

	// First batch
	batch.AddGet("key1")
	batch.AddGet("key2")
	assert.Equal(t, 2, batch.Size())

	batch.Clear()
	assert.Equal(t, 0, batch.Size())

	// Reuse for second batch
	batch.AddSet("key3", "value", time.Hour)
	batch.AddSet("key4", "value", time.Hour)
	assert.Equal(t, 2, batch.Size())
}

func TestRedisBatch_InitialCapacity(t *testing.T) {
	logger := zap.NewNop()
	batch := NewRedisBatch(logger)

	// Should have pre-allocated capacity
	assert.Equal(t, 0, len(batch.operations))
	assert.Equal(t, 0, len(batch.keys))

	// Can add operations up to capacity without reallocation
	for i := 0; i < 100; i++ {
		batch.AddGet("test:key")
	}

	assert.Equal(t, 100, batch.Size())
}

func TestRedisBatch_LargeNumberOfOperations(t *testing.T) {
	logger := zap.NewNop()
	batch := NewRedisBatch(logger)

	// Add 1000 operations
	for i := 0; i < 1000; i++ {
		batch.AddGet("test:key")
	}

	assert.Equal(t, 1000, batch.Size())
	assert.Len(t, batch.keys, 1000)
}

func TestRedisBatch_AddSet_DifferentTypes(t *testing.T) {
	logger := zap.NewNop()
	batch := NewRedisBatch(logger)

	// Different value types
	batch.AddSet("key:string", "string value", time.Hour)
	batch.AddSet("key:int", 123, time.Hour)
	batch.AddSet("key:bool", true, time.Hour)
	batch.AddSet("key:float", 3.14, time.Hour)

	assert.Equal(t, 4, batch.Size())
}

func TestRedisBatch_AddSet_DifferentExpirations(t *testing.T) {
	logger := zap.NewNop()
	batch := NewRedisBatch(logger)

	expirations := []time.Duration{
		1 * time.Minute,
		1 * time.Hour,
		24 * time.Hour,
		0, // No expiration
	}

	for i, exp := range expirations {
		batch.AddSet("test:key", "value", exp)
		assert.Equal(t, i+1, batch.Size())
	}

	assert.Equal(t, 4, batch.Size())
}

func TestRedisBatch_EmptyBatch(t *testing.T) {
	logger := zap.NewNop()
	batch := NewRedisBatch(logger)

	// Empty batch should have size 0
	assert.Equal(t, 0, batch.Size())

	// Clear on empty batch should not panic
	assert.NotPanics(t, func() {
		batch.Clear()
	})

	assert.Equal(t, 0, batch.Size())
}

func TestRedisBatch_AddGet_EmptyKey(t *testing.T) {
	logger := zap.NewNop()
	batch := NewRedisBatch(logger)

	batch.AddGet("")

	assert.Equal(t, 1, batch.Size())
	assert.Contains(t, batch.keys, "")
}

func TestRedisBatch_AddSet_EmptyKey(t *testing.T) {
	logger := zap.NewNop()
	batch := NewRedisBatch(logger)

	batch.AddSet("", "value", time.Hour)

	assert.Equal(t, 1, batch.Size())
	assert.Contains(t, batch.keys, "")
}

func TestRedisBatch_AddDel_EmptyKeys(t *testing.T) {
	logger := zap.NewNop()
	batch := NewRedisBatch(logger)

	batch.AddDel("", "")

	assert.Equal(t, 1, batch.Size())
	assert.Len(t, batch.keys, 2)
}

func TestRedisBatch_SequentialOperations(t *testing.T) {
	logger := zap.NewNop()
	batch := NewRedisBatch(logger)

	// Sequence: Get, Set, Delete, Get, Set
	batch.AddGet("seq:1")
	assert.Equal(t, 1, batch.Size())

	batch.AddSet("seq:2", "val", time.Hour)
	assert.Equal(t, 2, batch.Size())

	batch.AddDel("seq:3")
	assert.Equal(t, 3, batch.Size())

	batch.AddGet("seq:4")
	assert.Equal(t, 4, batch.Size())

	batch.AddSet("seq:5", "val", time.Hour)
	assert.Equal(t, 5, batch.Size())
}

func TestRedisBatch_DuplicateKeys(t *testing.T) {
	logger := zap.NewNop()
	batch := NewRedisBatch(logger)

	// Add same key multiple times
	batch.AddGet("duplicate:key")
	batch.AddGet("duplicate:key")
	batch.AddSet("duplicate:key", "value", time.Hour)

	assert.Equal(t, 3, batch.Size())
	assert.Len(t, batch.keys, 3)

	// Count occurrences
	count := 0
	for _, key := range batch.keys {
		if key == "duplicate:key" {
			count++
		}
	}
	assert.Equal(t, 3, count)
}

func TestRedisBatch_KeysTracking(t *testing.T) {
	logger := zap.NewNop()
	batch := NewRedisBatch(logger)

	keys := []string{"key1", "key2", "key3", "key4", "key5"}

	for _, key := range keys {
		batch.AddGet(key)
	}

	assert.Equal(t, len(keys), batch.Size())

	// Verify all keys are tracked
	for _, key := range keys {
		assert.Contains(t, batch.keys, key)
	}
}

func TestRedisBatch_NilLogger(t *testing.T) {
	// Creating batch with nil logger should not panic during construction
	assert.NotPanics(t, func() {
		batch := NewRedisBatch(nil)
		assert.NotNil(t, batch)
		batch.AddGet("test")
		batch.AddSet("test", "value", time.Hour)
		batch.AddDel("test")
		batch.Clear()
	})
}
