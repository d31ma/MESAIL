package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/smtp"
	"net/textproto"
	"strings"
	"testing"

	"mesail.dev/packages/dkim"
	"mesail.dev/packages/mailauth"
	"mesail.dev/packages/smtpclient/sink"
)

func freeAddress(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	listener.Close()
	return address
}

func edKey(t *testing.T) (ed25519.PrivateKey, string) {
	t.Helper()
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return private, "v=DKIM1; k=ed25519; p=" + base64.StdEncoding.EncodeToString(public)
}

func signedMessage(t *testing.T, private ed25519.PrivateKey) []byte {
	t.Helper()
	message := "From: ana@example.com\r\nTo: someone@outside.test\r\nSubject: Hello outside\r\n\r\nHello outside.\r\n"
	signed, err := dkim.Signer{}.Sign(dkim.SignRequest{Message: []byte(message), Key: dkim.Key{Domain: "example.com", Selector: "mesail", Signer: private}})
	if err != nil {
		t.Fatal(err)
	}
	return signed
}

func startSink(t *testing.T) *sink.Sink {
	t.Helper()
	server, err := sink.Listen(freeAddress(t), sink.Options{RejectRecipient: Refusal{LocalPart: "refused"}.Reject})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { server.Close() })
	return server
}

func listed(t *testing.T, server *sink.Sink, records map[string]string) []MessageReport {
	t.Helper()
	listing := Listing{Messages: server.Messages, Checker: mailauth.NewChecker(DkimResolver{Records: records})}
	recorder := httptest.NewRecorder()
	listing.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/messages", nil))
	var reports []MessageReport
	if err := json.Unmarshal(recorder.Body.Bytes(), &reports); err != nil {
		t.Fatalf("listing is not JSON: %v: %s", err, recorder.Body.String())
	}
	return reports
}

func TestSignedMessageIsListedWithAPassingDkimResult(t *testing.T) {
	private, record := edKey(t)
	server := startSink(t)
	if err := smtp.SendMail(server.Address(), nil, "ana@example.com", []string{"someone@outside.test"}, signedMessage(t, private)); err != nil {
		t.Fatal(err)
	}
	reports := listed(t, server, map[string]string{"mesail._domainkey.example.com": record})
	if len(reports) != 1 || reports[0].Recipients[0] != "someone@outside.test" {
		t.Fatalf("want one message to someone@outside.test, got %+v", reports)
	}
	want := DkimReport{Domain: "example.com", Selector: "mesail", Result: "pass"}
	if len(reports[0].Dkim) != 1 || reports[0].Dkim[0] != want {
		t.Fatalf("want DKIM %+v, got %+v", want, reports[0].Dkim)
	}
}

func TestSignatureWithAnotherKeyFails(t *testing.T) {
	private, _ := edKey(t)
	_, otherRecord := edKey(t)
	server := startSink(t)
	if err := smtp.SendMail(server.Address(), nil, "ana@example.com", []string{"someone@outside.test"}, signedMessage(t, private)); err != nil {
		t.Fatal(err)
	}
	reports := listed(t, server, map[string]string{"mesail._domainkey.example.com": otherRecord})
	if len(reports) != 1 || len(reports[0].Dkim) != 1 || reports[0].Dkim[0].Result == "pass" {
		t.Fatalf("want one failed DKIM result, got %+v", reports)
	}
}

func TestRefusedLocalPartGets550AndIsNotKept(t *testing.T) {
	server := startSink(t)
	err := smtp.SendMail(server.Address(), nil, "ana@example.com", []string{"refused@bb1.outside.test"}, []byte("Subject: x\r\n\r\nx\r\n"))
	var refused *textproto.Error
	if !errors.As(err, &refused) || refused.Code != 550 || !strings.HasPrefix(refused.Msg, "5.1.1 ") {
		t.Fatalf("want 550 5.1.1, got %v", err)
	}
	if reports := listed(t, server, nil); len(reports) != 0 {
		t.Fatalf("want no message kept, got %+v", reports)
	}
}

func TestEmptyListingIsAnEmptyArray(t *testing.T) {
	server := startSink(t)
	recorder := httptest.NewRecorder()
	Listing{Messages: server.Messages}.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/messages", nil))
	if strings.TrimSpace(recorder.Body.String()) != "[]" {
		t.Fatalf("want [], got %q", recorder.Body.String())
	}
}

func TestOtherLocalPartsAreAccepted(t *testing.T) {
	code, text := Refusal{LocalPart: "refused"}.Reject("refused-not@outside.test")
	if code != 0 || text != "" {
		t.Fatalf("want accepted, got %d %q", code, text)
	}
}

func TestRefusalIgnoresCase(t *testing.T) {
	code, _ := Refusal{LocalPart: "refused"}.Reject("Refused@outside.test")
	if code != 550 {
		t.Fatalf("want 550, got %d", code)
	}
}

func TestSettingsDefaults(t *testing.T) {
	settings, err := SettingsFromEnvironment(func(string) string { return "" })
	if err != nil {
		t.Fatal(err)
	}
	if settings.SmtpAddress != "0.0.0.0:2525" || settings.ListingAddress != "0.0.0.0:8025" || settings.RefusedLocalPart != "refused" || len(settings.DkimRecords) != 0 {
		t.Fatalf("unexpected defaults %+v", settings)
	}
}

func TestSettingsReadTheDkimRecords(t *testing.T) {
	environment := map[string]string{"RELAY_SINK_DKIM_RECORDS": `{"mesail._domainkey.example.com":"v=DKIM1; k=ed25519; p=AAAA"}`}
	settings, err := SettingsFromEnvironment(func(name string) string { return environment[name] })
	if err != nil {
		t.Fatal(err)
	}
	if settings.DkimRecords["mesail._domainkey.example.com"] != "v=DKIM1; k=ed25519; p=AAAA" {
		t.Fatalf("records not read: %+v", settings.DkimRecords)
	}
}

func TestSettingsRefuseBrokenDkimRecords(t *testing.T) {
	environment := map[string]string{"RELAY_SINK_DKIM_RECORDS": "not json"}
	_, err := SettingsFromEnvironment(func(name string) string { return environment[name] })
	if err == nil {
		t.Fatal("want an error for records that are not JSON")
	}
}
