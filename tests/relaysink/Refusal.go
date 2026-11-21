package main

import (
	"fmt"
	"strings"
)

type Refusal struct {
	LocalPart string
}

func (refusal Refusal) Reject(address string) (int, string) {
	localPart, _, _ := strings.Cut(address, "@")
	return refusal.answer(strings.EqualFold(localPart, refusal.LocalPart), address)
}

func (refusal Refusal) answer(refused bool, address string) (int, string) {
	if !refused {
		return 0, ""
	}
	return refusal.code(), fmt.Sprintf("5.1.1 <%s>: Recipient address rejected: no such user", address)
}

func (Refusal) code() int { return 550 }
