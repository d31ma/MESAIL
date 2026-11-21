//go:build blackbox

package blackbox

import (
	"net/http"
	"testing"
	"time"
)

type outboxCount struct {
	Documents int `json:"documents"`
}

type cleanupReport struct {
	Dropped *int  `json:"dropped"`
	More    *bool `json:"more"`
}

func twoSchedulerTicksAndSlack() time.Duration {
	return 150 * time.Second
}

func twoFastSchedulerTicksAndSlack() time.Duration {
	return 75 * time.Second
}

func readersBeyondTheInlineLimit() int {
	return 22
}

func mostRecipientsPerSend() int {
	return 20
}

func addedToConversation(conversationId string, people ...newPerson) map[string]any {
	message := groupMessage("jobs fan-out", uniqueClientId("fan-out"), people...)
	message["conversationId"] = conversationId
	return message
}

func domainServiceToken(t *testing.T, stack Stack, domainId string) string {
	t.Helper()
	var token struct {
		AccessToken string `json:"accessToken"`
	}
	serviceTokenReply(t, stack, serviceClientSecret(t), domainId).Expect(t, http.StatusOK).Decode(t, &token)
	return token.AccessToken
}

func channelsPending(t *testing.T, stack Stack) int {
	t.Helper()
	var count outboxCount
	stack.Call(t, http.MethodGet, "/api/v1/channels/outbox/pending", domainServiceToken(t, stack, "example.com"), nil).Expect(t, http.StatusOK).Decode(t, &count)
	return count.Documents
}

func renewingPersonTokens(t *testing.T, stack Stack, person newPerson) tokenSource {
	t.Helper()
	token := ""
	signedAt := time.Time{}
	return func() string {
		if time.Since(signedAt) > 45*time.Second {
			token = signIn(t, stack, person.Username, person.Password).AccessToken
			signedAt = time.Now()
		}
		return token
	}
}

func manyReaders(t *testing.T, stack Stack, adminToken string, count int) []newPerson {
	t.Helper()
	people := []newPerson{}
	for len(people) < count {
		people = append(people, changedPassword(t, stack, createPerson(t, stack, adminToken)))
	}
	return people
}

func awaitEveryReaderNotified(t *testing.T, stack Stack, people []newPerson, messageId string) {
	t.Helper()
	deadline := time.Now().Add(twoFastSchedulerTicksAndSlack())
	for _, person := range people {
		tokens := renewingPersonTokens(t, stack, person)
		stack.Eventually(t, time.Until(deadline), person.Username+"'s notification for message "+messageId, func() bool {
			_, found := notificationFor(t, stack, tokens(), messageId)
			return found
		})
	}
}

func TestFlowJobsChannelsOutboxRelayLeavesNothingPending(t *testing.T) {
	stack := NewStack(t)
	createPublicChannel(t, stack, fixture(t, stack, "ANA").AccessToken, freshPrefix())
	stack.Eventually(t, twoSchedulerTicksAndSlack(), "the channels outbox to have no pending document", func() bool {
		return channelsPending(t, stack) == 0
	})
}

func TestFlowJobsMailOutDeliveryRunsOnTheFastScheduler(t *testing.T) {
	stack := NewStack(t)
	ana := NewRenewingToken(t, stack, "ANA")
	sent := send(t, stack, ana.Get(t), emailTo("jobs-fast-delivery", uniqueClientId("fast delivery"), "eve@example.org", "otherDomain"), http.StatusCreated)
	started := time.Now()
	awaitEmailStatus(t, stack, ana, sent.Message.MessageId, "eve@example.org", "sent")
	if waited := time.Since(started); waited > twoFastSchedulerTicksAndSlack() {
		t.Fatalf("the email was sent after %s; scheduler-fast should send it within %s", waited, twoFastSchedulerTicksAndSlack())
	}
}

func TestFlowJobsNotifyFinishesAFanOutToMoreThanTwentyReaders(t *testing.T) {
	stack := NewStack(t)
	ana := fixtureTokens(t, stack, "ANA")
	readers := manyReaders(t, stack, ana(), readersBeyondTheInlineLimit())
	first := send(t, stack, ana(), groupMessage("jobs fan-out start", "start", readers[:mostRecipientsPerSend()]...), http.StatusCreated)
	sent := send(t, stack, ana(), addedToConversation(first.Message.ConversationId, readers[mostRecipientsPerSend():]...), http.StatusCreated)
	awaitEveryReaderNotified(t, stack, readers, sent.Message.MessageId)
}

func TestFlowJobsChangesCleanupAnswersFromAServiceToken(t *testing.T) {
	stack := NewStack(t)
	var report cleanupReport
	stack.Call(t, http.MethodPost, "/api/v1/messaging/jobs/changes-cleanup", installWideServiceToken(t, stack), nil).Expect(t, http.StatusOK).Decode(t, &report)
	if report.Dropped == nil || report.More == nil {
		t.Fatalf("want CleanupReport{dropped, more}, got dropped %v more %v", report.Dropped, report.More)
	}
}
