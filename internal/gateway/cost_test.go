package gateway

import (
	"testing"

	"opencode2api/internal/models"
	wire "opencode2api/internal/protocol"
)

func price(value float64) *float64 { return &value }

func TestPriceUsageSeparatesCachedTokens(t *testing.T) {
	cost := &models.Cost{
		Input: price(3), Output: price(15), CacheRead: price(0.3), CacheWrite: price(3.75),
	}
	usage := wire.Usage{Input: 1_000_000, Cached: 600_000, CacheCreation: 100_000, Output: 200_000}

	// 300k uncached input at $3 + 600k cache reads at $0.30 + 100k cache
	// writes at $3.75 + 200k output at $15, all per million tokens.
	want := 0.9 + 0.18 + 0.375 + 3.0
	if got := priceUsage(cost, usage); got < want-1e-9 || got > want+1e-9 {
		t.Fatalf("priceUsage = %v, want %v", got, want)
	}
}

func TestPriceUsageWithoutCachePrices(t *testing.T) {
	cost := &models.Cost{Input: price(2), Output: price(8)}
	// Cached tokens are still part of Input, so without a cache price they
	// are charged at the input rate rather than dropped.
	usage := wire.Usage{Input: 1_000_000, Cached: 400_000, Output: 100_000}
	want := 2.0 + 0.8
	if got := priceUsage(cost, usage); got < want-1e-9 || got > want+1e-9 {
		t.Fatalf("priceUsage = %v, want %v", got, want)
	}
}

func TestPriceUsageHandlesMissingInputs(t *testing.T) {
	if got := priceUsage(nil, wire.Usage{Input: 1000}); got != 0 {
		t.Fatalf("nil price = %v, want 0", got)
	}
	if got := priceUsage(&models.Cost{}, wire.Usage{Input: 1000, Output: 500}); got != 0 {
		t.Fatalf("empty price = %v, want 0", got)
	}
	// A usage that claims more cached than input must not go negative.
	cost := &models.Cost{Input: price(1), CacheRead: price(0.1)}
	if got := priceUsage(cost, wire.Usage{Input: 100, Cached: 500}); got < 0 {
		t.Fatalf("priceUsage = %v, want a non-negative cost", got)
	}
}
