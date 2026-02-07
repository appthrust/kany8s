package eks

import "time"

const (
	defaultRefreshBefore    = 5 * time.Minute
	defaultMaxRefresh       = 10 * time.Minute
	defaultFailureBackoff   = 30 * time.Second
	defaultImmediateRequeue = 15 * time.Second
)

type RequeuePolicy struct {
	RefreshBefore    time.Duration
	MaxRefresh       time.Duration
	FailureBackoff   time.Duration
	ImmediateRequeue time.Duration
}

func DefaultRequeuePolicy() RequeuePolicy {
	return RequeuePolicy{
		RefreshBefore:    defaultRefreshBefore,
		MaxRefresh:       defaultMaxRefresh,
		FailureBackoff:   defaultFailureBackoff,
		ImmediateRequeue: defaultImmediateRequeue,
	}
}

func (p RequeuePolicy) WithDefaults() RequeuePolicy {
	def := DefaultRequeuePolicy()
	if p.RefreshBefore <= 0 {
		p.RefreshBefore = def.RefreshBefore
	}
	if p.MaxRefresh <= 0 {
		p.MaxRefresh = def.MaxRefresh
	}
	if p.FailureBackoff <= 0 {
		p.FailureBackoff = def.FailureBackoff
	}
	if p.ImmediateRequeue <= 0 {
		p.ImmediateRequeue = def.ImmediateRequeue
	}
	return p
}

func ComputeNextRequeue(now, expiration time.Time, p RequeuePolicy) time.Duration {
	p = p.WithDefaults()
	if expiration.IsZero() {
		return p.FailureBackoff
	}

	reconcileAt := expiration.Add(-p.RefreshBefore)
	until := reconcileAt.Sub(now)
	if until <= 0 {
		return p.ImmediateRequeue
	}
	if until > p.MaxRefresh {
		return p.MaxRefresh
	}
	return until
}
