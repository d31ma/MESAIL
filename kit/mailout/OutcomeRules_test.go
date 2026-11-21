package mailout

import (
	"testing"
	"time"
)

func TestAHandOffNotFoundIsABounceForAnUnknownMailbox(t *testing.T) {
	want := Outcome{Status: "bounced", Transport: "mail-in", Code: 550, Enhanced: "5.1.1", Detail: "unknown recipient at mail-in"}
	equal(t, want, OutcomeRules{}.FromHandOff(404, nil))
}

func TestAHandOffUnavailableIsDeferred(t *testing.T) {
	outcome := OutcomeRules{}.FromHandOff(503, nil)
	equal(t, "deferred", outcome.Status)
	equal(t, 451, outcome.Code)
}

func TestAnUnknownMailboxAtMailInIsReportedBouncedToTheSender(t *testing.T) {
	w := newWorld(t, func(w *world) { w.answers = []int{404} })
	envelope := w.event("example.com", "alice@example.com", "nobody@example.org")
	w.mustSend(envelope)

	equal(t, QueueReport{Picked: 1, Bounced: 1}, w.run(time.Now()))
	equal(t, "bounced", w.item("example.com", envelope.EventId).State)
	equal(t, []string{
		"mail-out example.com nobody@example.org queued ",
		"mail-out example.com nobody@example.org bounced unknown recipient at mail-in",
	}, w.changes())
}
