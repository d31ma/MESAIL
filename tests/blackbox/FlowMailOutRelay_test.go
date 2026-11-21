//go:build blackbox

package blackbox

import (
	"cmp"
	"encoding/json"
	"net/http"
	"os"
	"slices"
	"strings"
	"testing"
	"time"
)

type relaySinkDkim struct {
	Domain   string `json:"domain"`
	Selector string `json:"selector"`
	Result   string `json:"result"`
}

type relaySinkMessage struct {
	From       string          `json:"from"`
	Recipients []string        `json:"recipients"`
	Dkim       []relaySinkDkim `json:"dkim"`
	Data       string          `json:"data"`
}

func relaySinkAddress(t *testing.T) string {
	t.Helper()
	address := os.Getenv("MESAIL_RELAY_SINK_ADDR")
	if address == "" {
		t.Skip("MESAIL_RELAY_SINK_ADDR is not set: this stack has no relay sink")
	}
	return address
}

func refusedLocalPart() string {
	return cmp.Or(os.Getenv("MESAIL_RELAY_SINK_REFUSED_LOCAL_PART"), "refused")
}

func relaySinkMessages(t *testing.T, address string) []relaySinkMessage {
	t.Helper()
	client := http.Client{Timeout: 10 * time.Second}
	response, err := client.Get("http://" + address + "/messages")
	if err != nil {
		t.Fatalf("relay sink listing: %v", err)
	}
	defer response.Body.Close()
	var messages []relaySinkMessage
	if err := json.NewDecoder(response.Body).Decode(&messages); err != nil {
		t.Fatalf("relay sink listing is not JSON (status %d): %v", response.StatusCode, err)
	}
	return messages
}

func relaySinkMessageTo(t *testing.T, address string, recipient string) (relaySinkMessage, bool) {
	t.Helper()
	messages := relaySinkMessages(t, address)
	index := slices.IndexFunc(messages, func(message relaySinkMessage) bool {
		return slices.Contains(message.Recipients, recipient)
	})
	if index < 0 {
		return relaySinkMessage{}, false
	}
	return messages[index], true
}

func TestFlowMailOutRelayToAnOutsideAddressIsDelivered(t *testing.T) {
	sink := relaySinkAddress(t)
	stack := NewStack(t)
	ana := NewRenewingToken(t, stack, "ANA")
	recipient := freshPrefix() + "@outside.test"
	sent := send(t, stack, ana.Get(t), emailTo("mailout-relay", "hello through the relay", recipient, "external"), http.StatusCreated)
	awaitEmailStatus(t, stack, ana, sent.Message.MessageId, recipient, "sent")
	message, found := relaySinkMessageTo(t, sink, recipient)
	if !found {
		t.Fatalf("the status is sent, but the relay sink has no message to %s", recipient)
	}
	if !strings.Contains(message.Data, "Subject: Blackbox mailout-relay") {
		t.Fatalf("the relayed message is not Ana's email:\n%s", message.Data)
	}
	want := relaySinkDkim{Domain: "example.com", Selector: "mesail", Result: "pass"}
	if !slices.Contains(message.Dkim, want) {
		t.Fatalf("want a DKIM signature %+v that passes, got %+v", want, message.Dkim)
	}
}

func TestFlowMailOutRelayRefusedByTheRelayIsReportedBack(t *testing.T) {
	sink := relaySinkAddress(t)
	stack := NewStack(t)
	ana := NewRenewingToken(t, stack, "ANA")
	recipient := refusedLocalPart() + "@" + freshPrefix() + ".outside.test"
	sent := send(t, stack, ana.Get(t), emailTo("mailout-relay-refused", "nobody gets this", recipient, "external"), http.StatusCreated)
	awaitEmailStatus(t, stack, ana, sent.Message.MessageId, recipient, "bounced")
	if _, found := relaySinkMessageTo(t, sink, recipient); found {
		t.Fatalf("the relay sink refused %s, yet it kept a message to it", recipient)
	}
}
