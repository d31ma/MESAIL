package main

import (
	"cmp"
	"encoding/json"
)

type Settings struct {
	SmtpAddress      string
	ListingAddress   string
	RefusedLocalPart string
	DkimRecords      map[string]string
}

func SettingsFromEnvironment(lookup func(string) string) (Settings, error) {
	defaults := Settings{}
	records := map[string]string{}
	err := json.Unmarshal([]byte(cmp.Or(lookup("RELAY_SINK_DKIM_RECORDS"), defaults.noRecords())), &records)
	return Settings{
		SmtpAddress:      cmp.Or(lookup("RELAY_SINK_SMTP_LISTEN"), defaults.smtpAddress()),
		ListingAddress:   cmp.Or(lookup("RELAY_SINK_LISTING_LISTEN"), defaults.listingAddress()),
		RefusedLocalPart: cmp.Or(lookup("RELAY_SINK_REFUSED_LOCAL_PART"), defaults.refusedLocalPart()),
		DkimRecords:      records,
	}, err
}

func (Settings) smtpAddress() string { return "0.0.0.0:2525" }

func (Settings) listingAddress() string { return "0.0.0.0:8025" }

func (Settings) refusedLocalPart() string { return "refused" }

func (Settings) noRecords() string { return "{}" }
