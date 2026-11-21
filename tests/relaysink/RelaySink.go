package main

import (
	"log"
	"net/http"
	"time"

	"mesail.dev/packages/mailauth"
	"mesail.dev/packages/smtpclient/sink"
)

type RelaySink struct {
	Settings Settings
}

func (relay RelaySink) Run() error {
	server, err := sink.Listen(relay.Settings.SmtpAddress, sink.Options{RejectRecipient: Refusal{LocalPart: relay.Settings.RefusedLocalPart}.Reject})
	if err != nil {
		return err
	}
	defer server.Close()
	log.Printf("relay sink: SMTP on %s, listing on %s, refuses local part %q, %d DKIM records", server.Address(), relay.Settings.ListingAddress, relay.Settings.RefusedLocalPart, len(relay.Settings.DkimRecords))
	listing := Listing{Messages: server.Messages, Checker: mailauth.NewChecker(DkimResolver{Records: relay.Settings.DkimRecords})}
	return relay.httpServer(listing).ListenAndServe()
}

func (relay RelaySink) httpServer(listing Listing) *http.Server {
	routes := http.NewServeMux()
	routes.Handle(relay.listingRoute(), listing)
	return &http.Server{Addr: relay.Settings.ListingAddress, Handler: routes, ReadHeaderTimeout: relay.headerTimeout()}
}

func (RelaySink) listingRoute() string { return "GET /messages" }

func (RelaySink) headerTimeout() time.Duration { return 10 * time.Second }
