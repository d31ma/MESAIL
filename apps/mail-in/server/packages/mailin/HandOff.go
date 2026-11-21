package mailin

import (
	"errors"

	"mesail.dev/packages/blobs"
)

type HandOff struct {
	Domains   Domains
	Addresses Addresses
	Blobs     blobs.Store
}

func (handOff HandOff) Received(envelope Envelope, domainId string) (Envelope, error) {
	if !envelope.AuthResults.Handoff {
		return envelope, nil
	}
	return handOff.copied(envelope, domainId)
}

func (handOff HandOff) copied(envelope Envelope, domainId string) (Envelope, error) {
	err := handOff.checkSender(EnvelopeRules{}.DomainOf(envelope.EnvelopeFrom))
	if err != nil {
		return Envelope{}, err
	}
	err = handOff.checkRecipient(envelope.Recipients[0])
	if err != nil {
		return Envelope{}, err
	}
	target := BlobKeys{}.HandOffCopy(domainId, envelope.RawBlobKey)
	err = handOff.Blobs.Copy(BlobKeys{}.FromPath(envelope.RawBlobKey), target)
	if err != nil {
		return Envelope{}, handOff.copyFailure(err)
	}
	envelope.RawBlobKey = target.Path()
	return envelope, nil
}

func (handOff HandOff) checkSender(domainId string) error {
	domain, found, err := handOff.Domains.Domain(domainId)
	if err != nil {
		return Unavailable{Operation: "directory"}
	}
	return handOff.hosted(domain, found)
}

func (handOff HandOff) checkRecipient(address string) error {
	known, err := handOff.Addresses.KnownAddress(address)
	if err != nil {
		return Unavailable{Operation: "directory"}
	}
	return handOff.mailbox(known)
}

func (HandOff) mailbox(known bool) error {
	if known {
		return nil
	}
	return Missing{}.UnknownRecipient()
}

func (HandOff) hosted(domain Domain, found bool) error {
	if found && domain.IsActive() {
		return nil
	}
	return Invalid{}.SenderDomainNotHosted()
}

func (HandOff) copyFailure(failure error) error {
	missing := blobs.NotFound{}
	if errors.As(failure, &missing) {
		return Invalid{}.NoRawMessage()
	}
	return Unavailable{Operation: "blob copy"}
}
