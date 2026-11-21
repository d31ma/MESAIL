package main

type MessageReport struct {
	From       string       `json:"from"`
	Recipients []string     `json:"recipients"`
	Dkim       []DkimReport `json:"dkim"`
	Data       string       `json:"data"`
}
