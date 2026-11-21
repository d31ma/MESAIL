package mailout

import (
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

type countingDocuments struct {
	*memoryDocuments
	writes atomic.Int64
}

func (documents *countingDocuments) CreateCollection(name string) error {
	documents.writes.Add(1)
	return documents.memoryDocuments.CreateCollection(name)
}

func (documents *countingDocuments) Insert(collection string, document any) (string, error) {
	documents.writes.Add(1)
	return documents.memoryDocuments.Insert(collection, document)
}

func (documents *countingDocuments) Patch(collection string, id string, change any, target any) (bool, error) {
	documents.writes.Add(1)
	return documents.memoryDocuments.Patch(collection, id, change, target)
}

func countedWorld(t *testing.T) (*world, *countingDocuments) {
	w := newWorld(t, nil)
	counted := &countingDocuments{memoryDocuments: w.documents}
	w.desk = DeskFactory{Environment: w.environment}.Desk(counted)
	w.desk.Gate = tokenGate{}
	return w, counted
}

func queueOutsideMail(w *world, count int) {
	for number := range count {
		w.mustSend(w.event("example.com", "alice@example.com", fmt.Sprintf("guest%d@partner.test", number)))
	}
}

func deliverCounted(t *testing.T, w *world, counted *countingDocuments) (QueueReport, int64) {
	t.Helper()
	before := counted.writes.Load()
	report := w.run(time.Now())
	return report, counted.writes.Load() - before
}

func deliverUntilDone(t *testing.T, w *world, counted *countingDocuments) (int, int, int64) {
	t.Helper()
	sent, calls, largest := 0, 0, int64(0)
	for more := true; more && calls < 50; calls++ {
		report, writes := deliverCounted(t, w, counted)
		sent, more, largest = sent+report.Sent, report.More, max(largest, writes)
	}
	return sent, calls, largest
}

func TestADeliverCallWithFiftyDueItemsStopsAtTheWriteCapAndAnswersMore(t *testing.T) {
	w, counted := countedWorld(t)
	queueOutsideMail(w, 50)

	report, writes := deliverCounted(t, w, counted)

	equal(t, true, report.More)
	equal(t, true, writes <= 25)
	equal(t, true, report.Picked > 0 && report.Picked < 50)
	equal(t, report.Picked, report.Sent)
}

func TestRepeatedDeliverCallsFinishTheQueueWithinTheWriteCapEachTime(t *testing.T) {
	w, counted := countedWorld(t)
	queueOutsideMail(w, 50)

	sent, calls, largest := deliverUntilDone(t, w, counted)

	equal(t, 50, sent)
	equal(t, 50, len(w.relay.Messages()))
	equal(t, true, largest <= 25)
	equal(t, true, calls > 1)
}

func TestADeliverCallStopsStartingItemsAfterTheTimeLimit(t *testing.T) {
	w, _ := countedWorld(t)
	queueOutsideMail(w, 3)
	moment := time.Now()
	w.desk.Run.Clock = func() time.Time {
		moment = moment.Add(15 * time.Second)
		return moment
	}

	report := w.run(time.Now())

	equal(t, QueueReport{Picked: 1, Sent: 1, More: true}, report)
}

func TestADeliverCallWithASmallQueueAnswersNoMore(t *testing.T) {
	w, counted := countedWorld(t)
	queueOutsideMail(w, 2)

	report, writes := deliverCounted(t, w, counted)

	equal(t, QueueReport{Picked: 2, Sent: 2, More: false}, report)
	equal(t, true, writes <= 25)
}
