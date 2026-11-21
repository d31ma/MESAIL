//go:build blackbox

package blackbox

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

type deviceOwner struct {
	person   newPerson
	token    string
	deviceId string
}

type envelopeReceipt struct {
	EnvelopeId string `json:"envelopeId"`
	ExpiresAt  string `json:"expiresAt"`
}

type storedEnvelope struct {
	EnvelopeId     string `json:"envelopeId"`
	SenderDeviceId string `json:"senderDeviceId"`
	SenderPersonId string `json:"senderPersonId"`
	Kind           string `json:"kind"`
	Ciphertext     string `json:"ciphertext"`
}

type envelopeList struct {
	Envelopes []storedEnvelope `json:"envelopes"`
	Next      string           `json:"next"`
}

type jobReport struct {
	Writes int  `json:"writes"`
	More   bool `json:"more"`
}

func randomBytes(t *testing.T, size int) []byte {
	t.Helper()
	raw := make([]byte, size)
	if _, err := rand.Read(raw); err != nil {
		t.Fatal(err)
	}
	return raw
}

func randomUuid(t *testing.T) string {
	t.Helper()
	raw := randomBytes(t, 16)
	raw[6] = (raw[6] & 0x0f) | 0x40
	raw[8] = (raw[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", raw[0:4], raw[4:6], raw[6:8], raw[8:10], raw[10:16])
}

func publicKeyText(t *testing.T) string {
	t.Helper()
	return base64.RawURLEncoding.EncodeToString(randomBytes(t, 32))
}

func ciphertextOf(t *testing.T, size int) string {
	t.Helper()
	return base64.RawURLEncoding.EncodeToString(randomBytes(t, size))
}

func ownerWithDevice(t *testing.T, stack Stack) deviceOwner {
	t.Helper()
	person := changedPassword(t, stack, createPerson(t, stack, fixture(t, stack, "ANA").AccessToken))
	token := signIn(t, stack, person.Username, person.Password).AccessToken
	deviceId := randomUuid(t)
	request := map[string]string{"deviceId": deviceId, "name": "Blackbox phone", "identityKey": publicKeyText(t), "agreementKey": publicKeyText(t)}
	stack.Call(t, http.MethodPost, "/api/v1/directory/devices", token, request).Expect(t, http.StatusCreated)
	owner := deviceOwner{person: person, token: token, deviceId: deviceId}
	removeDeviceAfterTest(t, stack, owner)
	return owner
}

func removeDeviceAfterTest(t *testing.T, stack Stack, owner deviceOwner) {
	t.Helper()
	t.Cleanup(func() {
		token := signIn(t, stack, owner.person.Username, owner.person.Password).AccessToken
		stack.Call(t, http.MethodDelete, "/api/v1/directory/devices/"+owner.deviceId, token, nil)
	})
}

func envelopeRequest(envelopeId string, senderDeviceId string, toDeviceId string, ciphertext string) map[string]any {
	return map[string]any{
		"envelopeId":     envelopeId,
		"senderDeviceId": senderDeviceId,
		"toDeviceIds":    []string{toDeviceId},
		"kind":           "message",
		"ciphertext":     ciphertext,
	}
}

func putEnvelopeReply(t *testing.T, stack Stack, token string, request map[string]any) Reply {
	t.Helper()
	return stack.Call(t, http.MethodPost, "/api/v1/devicebox/envelopes", token, request)
}

func putEnvelope(t *testing.T, stack Stack, token string, request map[string]any) envelopeReceipt {
	t.Helper()
	var receipt envelopeReceipt
	putEnvelopeReply(t, stack, token, request).Expect(t, http.StatusCreated).Decode(t, &receipt)
	return receipt
}

func boxPath(deviceId string) string {
	return "/api/v1/devicebox/devices/" + deviceId + "/envelopes"
}

func listBoxReply(t *testing.T, stack Stack, token string, deviceId string) Reply {
	t.Helper()
	return stack.Call(t, http.MethodGet, boxPath(deviceId), token, nil)
}

func listBox(t *testing.T, stack Stack, token string, deviceId string) envelopeList {
	t.Helper()
	var list envelopeList
	listBoxReply(t, stack, token, deviceId).Expect(t, http.StatusOK).Decode(t, &list)
	return list
}

func onlyEnvelope(t *testing.T, list envelopeList) storedEnvelope {
	t.Helper()
	if len(list.Envelopes) != 1 {
		t.Fatalf("want exactly one envelope, got %+v", list.Envelopes)
	}
	return list.Envelopes[0]
}

func installWideServiceToken(t *testing.T, stack Stack) string {
	t.Helper()
	var token struct {
		AccessToken string `json:"accessToken"`
	}
	serviceTokenReply(t, stack, serviceClientSecret(t), "*").Expect(t, http.StatusOK).Decode(t, &token)
	return token.AccessToken
}

func TestFlowDeviceboxEnvelopeReachesTheReceivingDevice(t *testing.T) {
	stack := NewStack(t)
	sender := ownerWithDevice(t, stack)
	receiver := ownerWithDevice(t, stack)
	envelopeId := randomUuid(t)
	ciphertext := ciphertextOf(t, 64)
	putEnvelope(t, stack, sender.token, envelopeRequest(envelopeId, sender.deviceId, receiver.deviceId, ciphertext))
	got := onlyEnvelope(t, listBox(t, stack, receiver.token, receiver.deviceId))
	want := storedEnvelope{EnvelopeId: envelopeId, SenderDeviceId: sender.deviceId, SenderPersonId: sender.person.Id, Kind: "message", Ciphertext: ciphertext}
	if got != want {
		t.Fatalf("want %+v, got %+v", want, got)
	}
}

func TestFlowDeviceboxAcknowledgedEnvelopeIsGone(t *testing.T) {
	stack := NewStack(t)
	sender := ownerWithDevice(t, stack)
	receiver := ownerWithDevice(t, stack)
	envelopeId := randomUuid(t)
	putEnvelope(t, stack, sender.token, envelopeRequest(envelopeId, sender.deviceId, receiver.deviceId, ciphertextOf(t, 64)))
	acknowledge := map[string][]string{"envelopeIds": {envelopeId}}
	stack.Call(t, http.MethodDelete, boxPath(receiver.deviceId), receiver.token, acknowledge).Expect(t, http.StatusNoContent)
	if left := listBox(t, stack, receiver.token, receiver.deviceId).Envelopes; len(left) != 0 {
		t.Fatalf("an acknowledged envelope is still listed: %+v", left)
	}
}

func TestFlowDeviceboxSamePutTwiceGivesTheSameReceipt(t *testing.T) {
	stack := NewStack(t)
	sender := ownerWithDevice(t, stack)
	receiver := ownerWithDevice(t, stack)
	request := envelopeRequest(randomUuid(t), sender.deviceId, receiver.deviceId, ciphertextOf(t, 64))
	first := putEnvelope(t, stack, sender.token, request)
	second := putEnvelope(t, stack, sender.token, request)
	if first != second {
		t.Fatalf("a repeated put gave another receipt: %+v and %+v", first, second)
	}
}

func TestFlowDeviceboxSameIdWithOtherContentIsAConflict(t *testing.T) {
	stack := NewStack(t)
	sender := ownerWithDevice(t, stack)
	receiver := ownerWithDevice(t, stack)
	envelopeId := randomUuid(t)
	putEnvelope(t, stack, sender.token, envelopeRequest(envelopeId, sender.deviceId, receiver.deviceId, ciphertextOf(t, 64)))
	other := envelopeRequest(envelopeId, sender.deviceId, receiver.deviceId, ciphertextOf(t, 64))
	putEnvelopeReply(t, stack, sender.token, other).Expect(t, http.StatusConflict)
}

func TestFlowDeviceboxAnotherPersonsBoxIsNotFound(t *testing.T) {
	stack := NewStack(t)
	sender := ownerWithDevice(t, stack)
	receiver := ownerWithDevice(t, stack)
	listBoxReply(t, stack, sender.token, receiver.deviceId).Expect(t, http.StatusNotFound)
}

func TestFlowDeviceboxSendingFromSomeoneElsesDeviceIsNotFound(t *testing.T) {
	stack := NewStack(t)
	sender := ownerWithDevice(t, stack)
	receiver := ownerWithDevice(t, stack)
	forged := envelopeRequest(randomUuid(t), receiver.deviceId, receiver.deviceId, ciphertextOf(t, 64))
	putEnvelopeReply(t, stack, sender.token, forged).Expect(t, http.StatusNotFound)
}

func TestFlowDeviceboxTooBigCiphertextIsRefused(t *testing.T) {
	stack := NewStack(t)
	sender := ownerWithDevice(t, stack)
	receiver := ownerWithDevice(t, stack)
	tooBig := envelopeRequest(randomUuid(t), sender.deviceId, receiver.deviceId, ciphertextOf(t, 262145))
	putEnvelopeReply(t, stack, sender.token, tooBig).Expect(t, http.StatusRequestEntityTooLarge)
}

func TestFlowDeviceboxExpiryJobLeavesFreshEnvelopes(t *testing.T) {
	stack := NewStack(t)
	sender := ownerWithDevice(t, stack)
	receiver := ownerWithDevice(t, stack)
	envelopeId := randomUuid(t)
	putEnvelope(t, stack, sender.token, envelopeRequest(envelopeId, sender.deviceId, receiver.deviceId, ciphertextOf(t, 64)))
	var report jobReport
	stack.Call(t, http.MethodPost, "/api/v1/devicebox/jobs/expire", installWideServiceToken(t, stack), nil).Expect(t, http.StatusOK).Decode(t, &report)
	if got := onlyEnvelope(t, listBox(t, stack, receiver.token, receiver.deviceId)); !strings.EqualFold(got.EnvelopeId, envelopeId) {
		t.Fatalf("the expiry job removed a fresh envelope; listed %+v", got)
	}
}
