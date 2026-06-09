package gateway

import (
	"fmt"
	"sync"

	"github.com/marcusguttenplan/sb/internal/ledger"
)

// Budget enforces daily spend limits using the token ledger.
// It caches the daily total in memory and re-reads the ledger periodically
// to stay accurate without hammering the disk.
type Budget struct {
	mu           sync.Mutex
	ledger       *ledger.Ledger
	limitUSD     float64
	warnUSD      float64
	cachedSpend  float64
	cachedDate   string
}

// BudgetStatus is the result of a budget check.
type BudgetStatus struct {
	// DailySpendUSD is the total spend so far today.
	DailySpendUSD float64
	// LimitUSD is the configured hard stop.
	LimitUSD float64
	// WarnUSD is the soft warning threshold.
	WarnUSD float64
	// Remaining is LimitUSD - DailySpendUSD. May be negative if over limit.
	Remaining float64
	// Exceeded is true when DailySpendUSD >= LimitUSD.
	Exceeded bool
	// Warned is true when DailySpendUSD >= WarnUSD (but not yet Exceeded).
	Warned bool
}

// ErrBudgetExceeded is returned by Check when the daily limit has been hit.
type ErrBudgetExceeded struct {
	SpendUSD float64
	LimitUSD float64
}

func (e *ErrBudgetExceeded) Error() string {
	return fmt.Sprintf("daily budget exceeded: spent $%.4f of $%.2f limit", e.SpendUSD, e.LimitUSD)
}

// NewBudget creates a Budget. If limitUSD is 0, enforcement is disabled.
func NewBudget(l *ledger.Ledger, limitUSD, warnUSD float64) *Budget {
	return &Budget{
		ledger:   l,
		limitUSD: limitUSD,
		warnUSD:  warnUSD,
	}
}

// Check returns the current budget status and returns ErrBudgetExceeded if over limit.
// It refreshes the spend total from the ledger on each call (fast: single file read).
func (b *Budget) Check() (*BudgetStatus, error) {
	if b.limitUSD == 0 {
		return &BudgetStatus{LimitUSD: 0, WarnUSD: b.warnUSD}, nil
	}

	spend, err := b.todaySpend()
	if err != nil {
		// Fail open: don't block on ledger read errors.
		return &BudgetStatus{LimitUSD: b.limitUSD, WarnUSD: b.warnUSD}, nil
	}

	status := &BudgetStatus{
		DailySpendUSD: spend,
		LimitUSD:      b.limitUSD,
		WarnUSD:       b.warnUSD,
		Remaining:     b.limitUSD - spend,
		Exceeded:      spend >= b.limitUSD,
		Warned:        b.warnUSD > 0 && spend >= b.warnUSD,
	}

	if status.Exceeded {
		return status, &ErrBudgetExceeded{SpendUSD: spend, LimitUSD: b.limitUSD}
	}
	return status, nil
}

// todaySpend returns the total USD spend for today from the ledger.
func (b *Budget) todaySpend() (float64, error) {
	s, err := b.ledger.TodaySummary()
	if err != nil {
		return 0, err
	}
	return s.TotalCostUSD, nil
}
