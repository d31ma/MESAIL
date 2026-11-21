//go:build blackbox

package blackbox

import (
	"net/http"
	"testing"
	"time"
)

type notification struct {
	NotificationId string `json:"notificationId"`
	Kind           string `json:"kind"`
	ConversationId string `json:"conversationId"`
	MessageId      string `json:"messageId"`
	ReadAt         string `json:"readAt"`
}

type notificationPage struct {
	Items []notification `json:"items"`
}

func notifyWait() time.Duration {
	return 30 * time.Second
}

func notificationsOf(t *testing.T, stack Stack, token string) []notification {
	t.Helper()
	var page notificationPage
	stack.Call(t, http.MethodGet, "/api/v1/notify/notifications?limit=100", token, nil).Expect(t, http.StatusOK).Decode(t, &page)
	return page.Items
}

func notificationFor(t *testing.T, stack Stack, token string, messageId string) (notification, bool) {
	t.Helper()
	for _, item := range notificationsOf(t, stack, token) {
		if item.MessageId == messageId {
			return item, true
		}
	}
	return notification{}, false
}

func awaitNotification(t *testing.T, stack Stack, renewing *RenewingToken, messageId string) notification {
	t.Helper()
	var found notification
	stack.Eventually(t, notifyWait(), "a notification for message "+messageId, func() bool {
		item, ok := notificationFor(t, stack, renewing.Get(t), messageId)
		found = item
		return ok
	})
	return found
}

func awaitRead(t *testing.T, stack Stack, renewing *RenewingToken, messageId string) {
	t.Helper()
	stack.Eventually(t, notifyWait(), "the notification for message "+messageId+" to be read", func() bool {
		item, _ := notificationFor(t, stack, renewing.Get(t), messageId)
		return item.ReadAt != ""
	})
}

func directMessageToBo(t *testing.T, stack Stack, text string) sendResult {
	t.Helper()
	return send(t, stack, fixture(t, stack, "ANA").AccessToken, toOne("notify", text, "bo@example.com", "chat", "member"), http.StatusCreated)
}

func markReadPath(notificationId string) string {
	return "/api/v1/notify/notifications/" + notificationId + "/read"
}

func TestFlowNotifyDirectMessageNotifiesTheRecipient(t *testing.T) {
	stack := NewStack(t)
	sent := directMessageToBo(t, stack, uniqueClientId("notify bo"))
	got := awaitNotification(t, stack, NewRenewingToken(t, stack, "BO"), sent.Message.MessageId)
	if got.Kind != "dm" || got.ConversationId != sent.Message.ConversationId || got.ReadAt != "" {
		t.Fatalf("want an unread dm notification in %s, got %+v", sent.Message.ConversationId, got)
	}
}

func TestFlowNotifyDirectMessageDoesNotNotifyTheAuthor(t *testing.T) {
	stack := NewStack(t)
	sent := directMessageToBo(t, stack, uniqueClientId("notify not ana"))
	awaitNotification(t, stack, NewRenewingToken(t, stack, "BO"), sent.Message.MessageId)
	if item, found := notificationFor(t, stack, fixture(t, stack, "ANA").AccessToken, sent.Message.MessageId); found {
		t.Fatalf("the author got a notification for her own message: %+v", item)
	}
}

func TestFlowNotifyMarkOneReadSetsReadAt(t *testing.T) {
	stack := NewStack(t)
	bo := NewRenewingToken(t, stack, "BO")
	sent := directMessageToBo(t, stack, uniqueClientId("notify mark read"))
	item := awaitNotification(t, stack, bo, sent.Message.MessageId)
	stack.Call(t, http.MethodPut, markReadPath(item.NotificationId), bo.Get(t), nil).Expect(t, http.StatusNoContent)
	if after, _ := notificationFor(t, stack, bo.Get(t), sent.Message.MessageId); after.ReadAt == "" {
		t.Fatalf("the notification is still unread after PUT .../read: %+v", after)
	}
}

func TestFlowNotifyReadingTheConversationMarksItRead(t *testing.T) {
	stack := NewStack(t)
	bo := NewRenewingToken(t, stack, "BO")
	sent := directMessageToBo(t, stack, uniqueClientId("notify conversation read"))
	awaitNotification(t, stack, bo, sent.Message.MessageId)
	path := "/api/v1/messaging/conversations/" + sent.Message.ConversationId + "/read"
	stack.Call(t, http.MethodPut, path, bo.Get(t), map[string]string{"messageId": sent.Message.MessageId}).Expect(t, http.StatusNoContent)
	awaitRead(t, stack, bo, sent.Message.MessageId)
}

func TestFlowNotifyAnotherPersonsNotificationIsNotFound(t *testing.T) {
	stack := NewStack(t)
	sent := directMessageToBo(t, stack, uniqueClientId("notify not eve"))
	item := awaitNotification(t, stack, NewRenewingToken(t, stack, "BO"), sent.Message.MessageId)
	stack.Call(t, http.MethodPut, markReadPath(item.NotificationId), stack.Token(t, "EVE"), nil).ExpectError(t, http.StatusNotFound, "not_found")
}

func TestFlowNotifyMutedConversationIsNotNotified(t *testing.T) {
	stack := NewStack(t)
	ana := NewRenewingToken(t, stack, "ANA")
	reader := changedPassword(t, stack, createPerson(t, stack, ana.Get(t)))
	readerToken := signIn(t, stack, reader.Username, reader.Password).AccessToken
	first := send(t, stack, ana.Get(t), toOne("notify muted", "before mute", reader.Username+"@example.com", "chat", "member"), http.StatusCreated)
	settings := map[string]any{"emailSummaryAfterMinutes": 0, "mutedConversationIds": []string{first.Message.ConversationId}}
	stack.Call(t, http.MethodPut, "/api/v1/notify/settings", readerToken, settings).Expect(t, http.StatusOK)
	second := send(t, stack, ana.Get(t), map[string]any{"clientMessageId": uniqueClientId("muted"), "conversationId": first.Message.ConversationId, "body": "after mute"}, http.StatusCreated)
	time.Sleep(notifyWait())
	if item, found := notificationFor(t, stack, signIn(t, stack, reader.Username, reader.Password).AccessToken, second.Message.MessageId); found {
		t.Fatalf("a muted conversation still notified: %+v", item)
	}
}
