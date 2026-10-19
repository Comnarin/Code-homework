package usecase

import "github.com/comnarin/foodstore/internal/domain"

// ClaimSource says which system decided a limited-product claim.
type ClaimSource string

const (
	ClaimSourceRedis    ClaimSource = "redis"
	ClaimSourceDatabase ClaimSource = "db"
)

// ClaimOutcome says what happened to a limited-product claim.
type ClaimOutcome string

const (
	ClaimClaimed  ClaimOutcome = "claimed"  // this request opened the window
	ClaimReused   ClaimOutcome = "reused"   // the same holder already had it
	ClaimRejected ClaimOutcome = "rejected" // another holder has it
	ClaimReleased ClaimOutcome = "released" // given back after a failed order
)

// Observer receives business events for metrics. Implementations must be safe
// for concurrent use and must never block or fail the request.
type Observer interface {
	OrderCalculated(q domain.Quote, member bool)
	LimitClaim(p domain.ProductID, src ClaimSource, out ClaimOutcome)
	// LimiterFallback records that Redis could not be used and the Postgres
	// guard decided instead.
	LimiterFallback(p domain.ProductID)
	ImageServed(fromStore bool)
}

// NopObserver ignores every event.
type NopObserver struct{}

func (NopObserver) OrderCalculated(domain.Quote, bool)                     {}
func (NopObserver) LimitClaim(domain.ProductID, ClaimSource, ClaimOutcome) {}
func (NopObserver) LimiterFallback(domain.ProductID)                       {}
func (NopObserver) ImageServed(bool)                                       {}
