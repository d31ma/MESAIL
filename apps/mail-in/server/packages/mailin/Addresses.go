package mailin

type Addresses interface {
	KnownAddress(address string) (bool, error)
}
