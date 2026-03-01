package pitcher

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/api/serviceerror"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestPitcher_NoConfig(t *testing.T) {
	p := New()
	ctx := context.Background()

	// Should return nil when no config exists
	_, err := p.MakePlay(ctx, "some.target", nil)
	assert.NoError(t, err)
}

func TestPitcher_FailAction(t *testing.T) {
	p := New()
	ctx := context.Background()

	p.Configure("test.target", PlayConfig{
		Play:  FailPlay(serviceerror.NewResourceExhausted(enumspb.RESOURCE_EXHAUSTED_CAUSE_SYSTEM_OVERLOADED, "test error")),
		Match: nil, // match any
	})

	_, err := p.MakePlay(ctx, "test.target", nil)
	require.Error(t, err)

	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.ResourceExhausted, st.Code())
}

func TestPitcher_FailActionDefaultError(t *testing.T) {
	p := New()
	ctx := context.Background()

	p.Configure("test.target", PlayConfig{
		Play:  FailPlay(serviceerror.NewInternal("test internal error")),
		Match: nil,
	})

	_, err := p.MakePlay(ctx, "test.target", nil)
	require.Error(t, err)

	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Internal, st.Code())
}

func TestPitcher_DelayAction(t *testing.T) {
	p := New()
	ctx := context.Background()

	p.Configure("test.target", PlayConfig{
		Play:  DelayPlay(50*time.Millisecond, 0),
		Match: nil,
	})

	start := time.Now()
	_, err := p.MakePlay(ctx, "test.target", nil)
	duration := time.Since(start)

	assert.NoError(t, err)
	assert.GreaterOrEqual(t, duration, 50*time.Millisecond)
	assert.Less(t, duration, 100*time.Millisecond) // sanity check
}

func TestPitcher_TimeoutAction(t *testing.T) {
	p := New()
	ctx := context.Background()

	p.Configure("test.target", PlayConfig{
		Play:  TimeoutPlay(10 * time.Millisecond),
		Match: nil,
	})

	_, err := p.MakePlay(ctx, "test.target", nil)
	require.Error(t, err)

	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.DeadlineExceeded, st.Code())
}

func TestPitcher_OneTimeUse(t *testing.T) {
	p := New()
	ctx := context.Background()

	// Configure a single play
	p.Configure("test.target", PlayConfig{
		Play:  FailPlay(serviceerror.NewUnavailable("test unavailable error")),
		Match: nil,
	})

	// First call should fail
	_, err1 := p.MakePlay(ctx, "test.target", nil)
	assert.Error(t, err1)

	// Second call should succeed (play was deleted after first use)
	_, err2 := p.MakePlay(ctx, "test.target", nil)
	assert.NoError(t, err2)
}

func TestPitcher_MultiplePlays(t *testing.T) {
	p := New()
	ctx := context.Background()

	// Configure two plays for the same target
	p.Configure("test.target", PlayConfig{
		Play:  FailPlay(serviceerror.NewUnavailable("test unavailable error")),
		Match: nil,
	})
	p.Configure("test.target", PlayConfig{
		Play:  FailPlay(serviceerror.NewInternal("test internal error")),
		Match: nil,
	})

	// First call should fail with first error
	_, err1 := p.MakePlay(ctx, "test.target", nil)
	require.Error(t, err1)
	st1, _ := status.FromError(err1)
	assert.Equal(t, codes.Unavailable, st1.Code())

	// Second call should fail with second error
	_, err2 := p.MakePlay(ctx, "test.target", nil)
	require.Error(t, err2)
	st2, _ := status.FromError(err2)
	assert.Equal(t, codes.Internal, st2.Code())

	// Third call should succeed (no more plays)
	_, err3 := p.MakePlay(ctx, "test.target", nil)
	assert.NoError(t, err3)
}

func TestPitcher_Reset(t *testing.T) {
	p := New()
	ctx := context.Background()

	p.Configure("test.target", PlayConfig{
		Play:  FailPlay(serviceerror.NewInternal("test internal error")),
		Match: nil,
	})

	// Should fail before reset
	_, err := p.MakePlay(ctx, "test.target", nil)
	assert.Error(t, err)

	// Reset
	p.Reset()

	// Should not fail after reset
	_, err = p.MakePlay(ctx, "test.target", nil)
	assert.NoError(t, err)
}

func TestPitcher_GlobalState(t *testing.T) {
	// Save and restore global state
	oldPitcher := Get()
	defer Set(oldPitcher)

	p := New()
	Set(p)

	assert.Equal(t, p, Get())

	Set(nil)
	assert.Nil(t, Get())
}

func TestPitcher_ContextCancellation(t *testing.T) {
	p := New()
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	p.Configure("test.target", PlayConfig{
		Play:  DelayPlay(1*time.Second, 0),
		Match: nil,
	})

	_, err := p.MakePlay(ctx, "test.target", nil)
	assert.Equal(t, context.Canceled, err)
}

func TestPitcher_DelayWithJitter(t *testing.T) {
	p := New()
	ctx := context.Background()

	// Measure several delays to check jitter is applied
	delays := make([]time.Duration, 10)
	for i := 0; i < len(delays); i++ {
		// Configure a new play for each iteration
		p.Configure("test.target", PlayConfig{
			Play:  DelayPlay(100*time.Millisecond, 0.5), // ±50% jitter
			Match: nil,
		})

		start := time.Now()
		_, err := p.MakePlay(ctx, "test.target", nil)
		delays[i] = time.Since(start)
		assert.NoError(t, err)
	}

	// All delays should be different due to jitter
	allSame := true
	for i := 1; i < len(delays); i++ {
		if delays[i] != delays[0] {
			allSame = false
			break
		}
	}
	assert.False(t, allSame, "Expected delays to vary due to jitter")

	// All delays should be within expected range (50-150ms with some tolerance)
	for _, d := range delays {
		assert.GreaterOrEqual(t, d, 40*time.Millisecond)
		assert.LessOrEqual(t, d, 160*time.Millisecond)
	}
}
