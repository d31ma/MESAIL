//go:build blackbox

package blackbox

import (
	"net/http"
	"testing"
	"time"
)

type deliveredMessage struct {
	MessageId  string `json:"messageId"`
	Deliveries []struct {
		Recipient string `json:"recipient"`
		Channel   string `json:"channel"`
		Status    string `json:"status"`
	} `json:"deliveries"`
}

func mailOutWait() time.Duration {
	return 150 * time.Second
}

func emailTo(name string, text string, address string, expectedKind string) map[string]any {
	body := toOne(name, text, address, "email", expectedKind)
	body["email"] = map[string]any{"subject": "Blackbox " + name}
	return body
}

func emailStatus(t *testing.T, stack Stack, token string, messageId string, recipient string) string {
	t.Helper()
	var message deliveredMessage
	stack.Call(t, http.MethodGet, "/api/v1/messaging/messages/"+messageId, token, nil).Expect(t, http.StatusOK).Decode(t, &message)
	for _, delivery := range message.Deliveries {
		if delivery.Recipient == recipient && delivery.Channel == "email" {
			return delivery.Status
		}
	}
	return ""
}

func awaitEmailStatus(t *testing.T, stack Stack, sender *RenewingToken, messageId string, recipient string, wanted string) {
	t.Helper()
	last := ""
	stack.Eventually(t, mailOutWait(), "the email to "+recipient+" to reach status "+wanted, func() bool {
		last = emailStatus(t, stack, sender.Get(t), messageId, recipient)
		return last == wanted
	})
}

func awaitEmailTried(t *testing.T, stack Stack, sender *RenewingToken, messageId string, recipient string) string {
	t.Helper()
	last := ""
	stack.Eventually(t, mailOutWait(), "the email to "+recipient+" to leave queued", func() bool {
		last = emailStatus(t, stack, sender.Get(t), messageId, recipient)
		return last != "" && last != "queued" && last != "sending"
	})
	return last
}

func TestFlowMailOutToAnotherHostedDomainIsSentAndArrives(t *testing.T) {
	stack := NewStack(t)
	ana := NewRenewingToken(t, stack, "ANA")
	eve := NewRenewingToken(t, stack, "EVE")
	text := uniqueClientId("Mail out to Eve")
	sent := send(t, stack, ana.Get(t), emailTo("mailout-hosted", text, "eve@example.org", "otherDomain"), http.StatusCreated)
	awaitEmailStatus(t, stack, ana, sent.Message.MessageId, "eve@example.org", "sent")
	stack.Eventually(t, mailOutWait(), "Eve to see Ana's email", func() bool {
		return externalConversationHasBody(t, stack, eve.Get(t), text)
	})
}

func TestFlowMailOutToAnUnknownMailboxInAHostedDomainIsReportedBounced(t *testing.T) {
	stack := NewStack(t)
	ana := NewRenewingToken(t, stack, "ANA")
	recipient := freshPrefix() + "@example.org"
	sent := send(t, stack, ana.Get(t), emailTo("mailout-unknown", "nobody reads this", recipient, "otherDomain"), http.StatusCreated)
	awaitEmailStatus(t, stack, ana, sent.Message.MessageId, recipient, "bounced")
}

func TestFlowMailOutToAnOutsideAddressIsTried(t *testing.T) {
	stack := NewStack(t)
	ana := NewRenewingToken(t, stack, "ANA")
	recipient := freshPrefix() + "@outside.test"
	sent := send(t, stack, ana.Get(t), emailTo("mailout-outside", "hello outside", recipient, "external"), http.StatusCreated)
	if status := awaitEmailTried(t, stack, ana, sent.Message.MessageId, recipient); status != "sent" && status != "deferred" {
		t.Fatalf("want sent (a relay is set) or deferred (no relay), got %s", status)
	}
}
