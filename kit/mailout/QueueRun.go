package mailout

import (
	"errors"
	"time"
)

type QueueRun struct {
	Registry Registry
	Queue    Queue
	Delivery Delivery
	Writes   WriteCounter
	Batch    int
	Clock    func() time.Time
}

func (QueueRun) writeLimit() int { return 25 }

func (QueueRun) mostWritesPerItem() int { return 7 }

func (QueueRun) timeLimit() time.Duration { return 20 * time.Second }

func (run QueueRun) Run(now time.Time) (QueueReport, error) {
	entries, err := run.Registry.Collections(run.Queue.Base())
	if err != nil {
		return QueueReport{}, err
	}
	due, err := run.dueIn(entries, now)
	if err != nil {
		return QueueReport{}, err
	}
	counts, started, err := run.deliverAll(due[:min(len(due), run.Batch)], now)
	return run.report(started, counts, len(due)), err
}

func (run QueueRun) dueIn(entries []CollectionEntry, now time.Time) ([]Item, error) {
	due := []Item{}
	for _, entry := range entries {
		items, err := run.Queue.Due(entry.DomainId, now)
		if err != nil {
			return nil, err
		}
		due = append(due, items...)
	}
	return due, nil
}

func (run QueueRun) deliverAll(due []Item, now time.Time) (map[string]int, int, error) {
	firstWrite, deadline := run.Writes.Written(), run.Clock().Add(run.timeLimit())
	counts, started := map[string]int{}, 0
	failures := []error{}
	for _, item := range due {
		state, begun, err := run.deliverWithRoom(item, now, firstWrite, deadline)
		counts[state], started = counts[state]+1, started+begun
		failures = append(failures, err)
	}
	return counts, started, errors.Join(failures...)
}

func (run QueueRun) deliverWithRoom(item Item, now time.Time, firstWrite int, deadline time.Time) (string, int, error) {
	if run.Writes.Written()-firstWrite+run.mostWritesPerItem() > run.writeLimit() || !run.Clock().Before(deadline) {
		return "", 0, nil
	}
	done, err := run.Delivery.Deliver(item, now)
	return done.State, 1, err
}

func (QueueRun) report(started int, counts map[string]int, due int) QueueReport {
	states := States{}
	return QueueReport{Picked: started, Sent: counts[states.Sent()], Deferred: counts[states.Deferred()], Bounced: counts[states.Bounced()], Expired: counts[states.Expired()], Failed: counts[states.Failed()], More: started < due}
}
