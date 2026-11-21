//go:build blackbox

package blackbox

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"strings"
	"testing"
	"time"
)

type TotpCode struct {
	secret []byte
}

func NewTotpCode(t *testing.T, secretText string) TotpCode {
	t.Helper()
	cleaned := strings.TrimRight(strings.ToUpper(strings.TrimSpace(secretText)), "=")
	secret, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(cleaned)
	if err != nil {
		t.Fatalf("the 2-step secret is not base32: %v", err)
	}
	return TotpCode{secret: secret}
}

func (code TotpCode) At(moment time.Time) string {
	return code.forStep(code.step(moment))
}

func (code TotpCode) StepsBefore(moment time.Time, steps int64) string {
	return code.forStep(code.step(moment) - steps)
}

func (code TotpCode) NextStepStart(moment time.Time) time.Time {
	return time.Unix((code.step(moment)+1)*code.periodSeconds(), 0)
}

func (code TotpCode) step(moment time.Time) int64 {
	return moment.Unix() / code.periodSeconds()
}

func (code TotpCode) forStep(step int64) string {
	counter := make([]byte, code.counterBytes())
	binary.BigEndian.PutUint64(counter, uint64(step))
	signer := hmac.New(sha1.New, code.secret)
	signer.Write(counter)
	return code.truncated(signer.Sum(nil))
}

func (code TotpCode) truncated(digest []byte) string {
	offset := digest[len(digest)-1] & code.offsetMask()
	value := binary.BigEndian.Uint32(digest[offset:]) & code.valueMask()
	return fmt.Sprintf("%06d", value%code.modulus())
}

func (TotpCode) periodSeconds() int64 { return 30 }

func (TotpCode) counterBytes() int { return 8 }

func (TotpCode) offsetMask() byte { return 0x0f }

func (TotpCode) valueMask() uint32 { return 0x7fffffff }

func (TotpCode) modulus() uint32 { return 1000000 }
