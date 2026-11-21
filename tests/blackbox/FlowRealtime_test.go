//go:build blackbox

package blackbox

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

type tokenSource func() string

type streamTicket struct {
	Ticket string `json:"ticket"`
}

func realtimeWait() time.Duration {
	return 75 * time.Second
}

func socketPath() string {
	return "/api/v1/realtime/socket"
}

func fixtureTokens(t *testing.T, stack Stack, name string) tokenSource {
	t.Helper()
	renewing := NewRenewingToken(t, stack, name)
	return func() string { return renewing.Get(t) }
}

func personTokens(t *testing.T, stack Stack, person newPerson) tokenSource {
	t.Helper()
	return func() string { return signIn(t, stack, person.Username, person.Password).AccessToken }
}

func expectSocketEvent(t *testing.T, client *SocketClient, wanted string) SocketEvent {
	t.Helper()
	event, open := client.NextEvent(t, 10*time.Second)
	if !open || event.Type != wanted {
		t.Fatalf("want a %s frame, got %+v (socket open %v, close code %d)", wanted, event, open, client.CloseCode())
	}
	return event
}

func connectSocket(t *testing.T, stack Stack, tokens tokenSource, deviceId string) *SocketClient {
	t.Helper()
	client := DialSocket(t, stack.baseUrl, socketPath())
	expectSocketEvent(t, client, "hello")
	client.SendJson(t, authFrame(tokens(), deviceId))
	expectSocketEvent(t, client, "auth.accepted")
	return client
}

func awaitSocketEvent(t *testing.T, client *SocketClient, tokens tokenSource, matches func(SocketEvent) bool) SocketEvent {
	t.Helper()
	deadline := time.Now().Add(realtimeWait())
	for time.Now().Before(deadline) {
		event, open := client.NextEvent(t, time.Until(deadline))
		answerReauth(t, client, tokens, event)
		if !open || matches(event) {
			return expectOpen(t, client, event, open)
		}
	}
	t.Fatalf("no matching socket frame within %s", realtimeWait())
	return SocketEvent{}
}

func answerReauth(t *testing.T, client *SocketClient, tokens tokenSource, event SocketEvent) {
	t.Helper()
	if event.Type == "reauth" {
		client.SendJson(t, authFrame(tokens(), ""))
	}
}

func expectOpen(t *testing.T, client *SocketClient, event SocketEvent, open bool) SocketEvent {
	t.Helper()
	if !open {
		t.Fatalf("the socket closed with code %d before the frame came", client.CloseCode())
	}
	return event
}

func messageCreated(messageId string) func(SocketEvent) bool {
	return func(event SocketEvent) bool {
		return event.Type == "message.created" && event.MessageId == messageId
	}
}

func refusalOf(t *testing.T, client *SocketClient) string {
	t.Helper()
	event, open := client.NextEvent(t, 15*time.Second)
	if open && event.Type != "auth.expired" {
		return refusalOf(t, client)
	}
	return fmt.Sprintf("auth.expired state %q, close code %d", event.State, client.CloseCode())
}

func authFrame(token string, deviceId string) map[string]string {
	frame := map[string]string{"type": "auth", "token": token}
	if deviceId != "" {
		frame["deviceId"] = deviceId
	}
	return frame
}

func newTicket(t *testing.T, stack Stack, token string) string {
	t.Helper()
	var ticket streamTicket
	stack.Call(t, http.MethodPost, "/api/v1/realtime/tickets", token, nil).Expect(t, http.StatusOK).Decode(t, &ticket)
	return ticket.Ticket
}

func streamSays(t *testing.T, stack Stack, ticket string, wanted func(line string) bool) bool {
	t.Helper()
	deadline, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(deadline, http.MethodGet, stack.baseUrl+"/api/v1/realtime/stream?ticket="+ticket, nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	return anyLine(bufio.NewScanner(response.Body), wanted)
}

func anyLine(lines *bufio.Scanner, wanted func(line string) bool) bool {
	for lines.Scan() {
		if wanted(lines.Text()) {
			return true
		}
	}
	return false
}

func acceptedLine(line string) bool {
	return strings.HasPrefix(line, "data:") && strings.Contains(line, `"auth.accepted"`)
}

func refusedLine(line string) bool {
	return line == "event: close" || (strings.HasPrefix(line, "data:") && strings.Contains(line, `"auth.expired"`))
}

func TestFlowRealtimeConnectedClientGetsANewMessage(t *testing.T) {
	stack := NewStack(t)
	bo := fixtureTokens(t, stack, "BO")
	client := connectSocket(t, stack, bo, "")
	sent := directMessageToBo(t, stack, uniqueClientId("realtime live"))
	event := awaitSocketEvent(t, client, bo, messageCreated(sent.Message.MessageId))
	if event.ConversationId != sent.Message.ConversationId {
		t.Fatalf("want conversation %s, got %+v", sent.Message.ConversationId, event)
	}
}

func TestFlowRealtimeClientGetsNewMessagesAfterReconnecting(t *testing.T) {
	stack := NewStack(t)
	bo := fixtureTokens(t, stack, "BO")
	connectSocket(t, stack, bo, "").Close()
	client := connectSocket(t, stack, bo, "")
	sent := directMessageToBo(t, stack, uniqueClientId("realtime reconnected"))
	awaitSocketEvent(t, client, bo, messageCreated(sent.Message.MessageId))
}

func TestFlowRealtimeMessageMissedWhileDisconnectedComesWithSync(t *testing.T) {
	stack := NewStack(t)
	bo := fixtureTokens(t, stack, "BO")
	first := directMessageToBo(t, stack, uniqueClientId("realtime before drop"))
	conversationId := first.Message.ConversationId
	cursor := ""
	stack.Eventually(t, 15*time.Second, "a change id for the conversation", func() bool {
		cursor = summaryOf(t, stack, bo(), conversationId).LastChangeId
		return cursor != ""
	})
	connectSocket(t, stack, bo, "").Close()
	missed := replyIn(t, stack, fixture(t, stack, "ANA").AccessToken, conversationId, uniqueClientId("realtime while away"))
	connectSocket(t, stack, bo, "")
	stack.Eventually(t, 15*time.Second, "sync to return the missed message", func() bool {
		_, count := messageNamed(syncOne(t, stack, bo(), conversationId, cursor), missed.Message.MessageId)
		return count == 1
	})
}

func TestFlowRealtimeSocketWithABadTokenIsRefused(t *testing.T) {
	stack := NewStack(t)
	client := DialSocket(t, stack.baseUrl, socketPath())
	expectSocketEvent(t, client, "hello")
	client.SendJson(t, authFrame("not-a-token", ""))
	if refusal := refusalOf(t, client); !strings.Contains(refusal, "4401") {
		t.Fatalf("want auth.expired 4401 or close 4401, got %q", refusal)
	}
}

func TestFlowRealtimeStreamTicketWorksOnce(t *testing.T) {
	stack := NewStack(t)
	ticket := newTicket(t, stack, fixture(t, stack, "BO").AccessToken)
	if !streamSays(t, stack, ticket, acceptedLine) {
		t.Fatal("the event stream did not accept a new ticket")
	}
	if !streamSays(t, stack, ticket, refusedLine) {
		t.Fatal("the event stream accepted a used ticket")
	}
}

func TestFlowRealtimeDeviceboxEnvelopeIsPushed(t *testing.T) {
	stack := NewStack(t)
	sender := ownerWithDevice(t, stack)
	receiver := ownerWithDevice(t, stack)
	tokens := personTokens(t, stack, receiver.person)
	client := connectSocket(t, stack, tokens, receiver.deviceId)
	putEnvelope(t, stack, sender.token, envelopeRequest(randomUuid(t), sender.deviceId, receiver.deviceId, ciphertextOf(t, 64)))
	awaitSocketEvent(t, client, tokens, func(event SocketEvent) bool { return event.Type == "devicebox.waiting" })
}
