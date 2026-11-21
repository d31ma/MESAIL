package main

type DkimReport struct {
	Domain   string `json:"domain"`
	Selector string `json:"selector"`
	Result   string `json:"result"`
}
