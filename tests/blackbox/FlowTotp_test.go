//go:build blackbox

package blackbox

import (
	"net/http"
	"testing"
	"time"
)

type totpSetup struct {
	Secret   string `json:"secret"`
	SetupKey string `json:"setupKey"`
}

type totpStatus struct {
	Enabled bool `json:"enabled"`
}

type totpPerson struct {
	person newPerson
	code   TotpCode
}

func startTotpSetup(t *testing.T, stack Stack) (newPerson, string, totpSetup) {
	t.Helper()
	person := changedPassword(t, stack, createPerson(t, stack, fixture(t, stack, "ANA").AccessToken))
	token := signIn(t, stack, person.Username, person.Password).AccessToken
	var setup totpSetup
	request := map[string]string{"currentPassword": person.Password}
	stack.Call(t, http.MethodPost, "/api/v1/identity/totp/setup", token, request).Expect(t, http.StatusOK).Decode(t, &setup)
	return person, token, setup
}

func confirmTotpReply(t *testing.T, stack Stack, token string, setupKey string, code string) Reply {
	t.Helper()
	request := map[string]string{"setupKey": setupKey, "code": code}
	return stack.Call(t, http.MethodPost, "/api/v1/identity/totp/confirm", token, request)
}

func personWithTotp(t *testing.T, stack Stack) totpPerson {
	t.Helper()
	person, token, setup := startTotpSetup(t, stack)
	code := NewTotpCode(t, setup.Secret)
	confirmTotpReply(t, stack, token, setup.SetupKey, code.At(time.Now())).Expect(t, http.StatusNoContent)
	return totpPerson{person: person, code: code}
}

func totpStatusOf(t *testing.T, stack Stack, token string) totpStatus {
	t.Helper()
	var status totpStatus
	stack.Call(t, http.MethodGet, "/api/v1/identity/totp", token, nil).Expect(t, http.StatusOK).Decode(t, &status)
	return status
}

func signInWithCodeReply(t *testing.T, stack Stack, person newPerson, code string) Reply {
	t.Helper()
	request := map[string]string{"login": person.Username, "password": person.Password, "oneTimeCode": code}
	return stack.Call(t, http.MethodPost, "/api/v1/identity/session", "", request)
}

func freshTotpStep(t *testing.T, code TotpCode) string {
	t.Helper()
	time.Sleep(time.Until(code.NextStepStart(time.Now())) + time.Second)
	return code.At(time.Now())
}

func TestFlowTotpStatusIsOnAfterConfirm(t *testing.T) {
	stack := NewStack(t)
	enrolled := personWithTotp(t, stack)
	var signedIn session
	signInWithCodeReply(t, stack, enrolled.person, freshTotpStep(t, enrolled.code)).Expect(t, http.StatusOK).Decode(t, &signedIn)
	if !totpStatusOf(t, stack, signedIn.AccessToken).Enabled {
		t.Fatal("GET /identity/totp says the 2-step code is off after a confirmed setup")
	}
}

func TestFlowTotpSignInWithTheRightCodeSucceeds(t *testing.T) {
	stack := NewStack(t)
	enrolled := personWithTotp(t, stack)
	var signedIn session
	signInWithCodeReply(t, stack, enrolled.person, freshTotpStep(t, enrolled.code)).Expect(t, http.StatusOK).Decode(t, &signedIn)
	if signedIn.AccessToken == "" {
		t.Fatalf("a sign-in with the right code gave no access token: %+v", signedIn)
	}
}

func TestFlowTotpSignInWithoutACodeIsRefused(t *testing.T) {
	stack := NewStack(t)
	enrolled := personWithTotp(t, stack)
	signInReply(t, stack, enrolled.person.Username, enrolled.person.Password).ExpectError(t, http.StatusUnauthorized, "unauthenticated")
}

func TestFlowTotpSignInWithAWrongCodeIsRefused(t *testing.T) {
	stack := NewStack(t)
	enrolled := personWithTotp(t, stack)
	oldCode := enrolled.code.StepsBefore(time.Now(), 10)
	signInWithCodeReply(t, stack, enrolled.person, oldCode).ExpectError(t, http.StatusUnauthorized, "unauthenticated")
}

func TestFlowTotpReusedCodeIsRefused(t *testing.T) {
	stack := NewStack(t)
	enrolled := personWithTotp(t, stack)
	code := freshTotpStep(t, enrolled.code)
	signInWithCodeReply(t, stack, enrolled.person, code).Expect(t, http.StatusOK)
	signInWithCodeReply(t, stack, enrolled.person, code).ExpectError(t, http.StatusUnauthorized, "unauthenticated")
}

func TestFlowTotpConfirmWithAWrongCodeIsRefused(t *testing.T) {
	stack := NewStack(t)
	_, token, setup := startTotpSetup(t, stack)
	wrongCode := NewTotpCode(t, setup.Secret).StepsBefore(time.Now(), 10)
	confirmTotpReply(t, stack, token, setup.SetupKey, wrongCode).ExpectError(t, http.StatusBadRequest, "invalid_code")
	if totpStatusOf(t, stack, token).Enabled {
		t.Fatal("a refused confirm turned the 2-step code on")
	}
}
