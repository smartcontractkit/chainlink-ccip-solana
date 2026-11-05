package devenv

import (
	"context"
	"math"
	"sync"
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewSignerPool_Valid(t *testing.T) {
	pool, err := NewSignerPool(5)
	require.NoError(t, err)
	require.Len(t, pool.PrivateKeys(), 5)

	// All keys should be unique.
	seen := make(map[solana.PublicKey]struct{})
	for _, k := range pool.PrivateKeys() {
		pub := k.PublicKey()
		_, dup := seen[pub]
		assert.False(t, dup, "duplicate key generated")
		seen[pub] = struct{}{}
	}
}

func TestNewSignerPool_InvalidSize(t *testing.T) {
	_, err := NewSignerPool(0)
	require.Error(t, err)

	_, err = NewSignerPool(-1)
	require.Error(t, err)

	_, err = NewSignerPool(MaxSignerPoolSize + 1)
	require.Error(t, err)
}

func TestNextUser_RoundRobin(t *testing.T) {
	const n = 3
	pool, err := NewSignerPool(n)
	require.NoError(t, err)

	keys := pool.PrivateKeys()
	// Call 3*N times — should cycle through keys in order.
	for cycle := range 3 {
		for i := range n {
			got := pool.NextUser()
			assert.Equal(t, keys[i].PublicKey(), got.PublicKey(),
				"cycle %d, index %d", cycle, i)
		}
	}
}

func TestNextUser_Concurrent(t *testing.T) {
	pool, err := NewSignerPool(5)
	require.NoError(t, err)

	validKeys := make(map[solana.PublicKey]struct{})
	for _, k := range pool.PrivateKeys() {
		validKeys[k.PublicKey()] = struct{}{}
	}

	const goroutines = 100
	const callsPerGoroutine = 1000

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for range goroutines {
		go func() {
			defer wg.Done()
			for range callsPerGoroutine {
				key := pool.NextUser()
				pub := key.PublicKey()
				if _, ok := validKeys[pub]; !ok {
					t.Errorf("NextUser returned key not in pool: %s", pub)
					return
				}
			}
		}()
	}
	wg.Wait()
}

func TestNextUser_Wrap(t *testing.T) {
	pool, err := NewSignerPool(3)
	require.NoError(t, err)

	// Set index near uint32 max to test wraparound.
	pool.index.Store(math.MaxUint32 - 1)

	// These calls should not panic.
	k1 := pool.NextUser() // counter becomes MaxUint32, effective idx = MaxUint32-1
	k2 := pool.NextUser() // counter wraps to 0, effective idx = MaxUint32
	k3 := pool.NextUser() // counter becomes 1, effective idx = 0

	keys := pool.PrivateKeys()
	assert.Equal(t, keys[(math.MaxUint32-1)%3].PublicKey(), k1.PublicKey())
	assert.Equal(t, keys[math.MaxUint32%3].PublicKey(), k2.PublicKey())
	assert.Equal(t, keys[0].PublicKey(), k3.PublicKey()) // (MaxUint32+1) % 3 == 0
}

func TestFindKey_Found(t *testing.T) {
	pool, err := NewSignerPool(3)
	require.NoError(t, err)

	target := pool.PrivateKeys()[1]
	found, ok := pool.FindKey(target.PublicKey())
	require.True(t, ok)
	require.Equal(t, target.PublicKey(), found.PublicKey())
}

func TestFindKey_NotFound(t *testing.T) {
	pool, err := NewSignerPool(3)
	require.NoError(t, err)

	randomKey, err := solana.NewRandomPrivateKey()
	require.NoError(t, err)

	found, ok := pool.FindKey(randomKey.PublicKey())
	assert.False(t, ok)
	assert.Nil(t, found)
}

func TestInitSignerPool_AlreadyInitialized(t *testing.T) {
	pool, err := NewSignerPool(3)
	require.NoError(t, err)

	c := &Chain{signerPool: pool}
	err = c.InitSignerPool(context.Background(), 5)
	require.Error(t, err)
	require.Contains(t, err.Error(), "already initialized")
}
