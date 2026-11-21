package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"mesail.dev/packages/mailauth"
	"mesail.dev/packages/smtpclient/sink"
)

type Listing struct {
	Messages func() []sink.Received
	Checker  mailauth.Checker
}

func (listing Listing) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	writer.Header().Set("Content-Type", "application/json")
	listing.encode(writer, listing.reports(request.Context()))
}

func (listing Listing) encode(writer http.ResponseWriter, reports []MessageReport) {
	err := json.NewEncoder(writer).Encode(reports)
	if err != nil {
		log.Printf("relay sink: listing not written: %v", err)
	}
}

func (listing Listing) reports(ctx context.Context) []MessageReport {
	reports := []MessageReport{}
	for _, received := range listing.Messages() {
		reports = append(reports, listing.report(ctx, received))
	}
	return reports
}

func (listing Listing) report(ctx context.Context, received sink.Received) MessageReport {
	results := listing.Checker.Check(ctx, mailauth.Input{Raw: received.Data, MailFrom: received.From, ReceivedAt: time.Now()})
	return MessageReport{From: received.From, Recipients: received.Recipients, Dkim: listing.dkim(results.Dkim), Data: string(received.Data)}
}

func (listing Listing) dkim(results []mailauth.DkimResult) []DkimReport {
	reports := []DkimReport{}
	for _, result := range results {
		reports = append(reports, DkimReport{Domain: result.Domain, Selector: result.Selector, Result: result.Result})
	}
	return reports
}
