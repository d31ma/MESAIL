package mailin

import "slices"

type DirectoryAnswer struct {
	Code string `json:"code"`
}

func (answer DirectoryAnswer) IsUnknownAddress() bool {
	return slices.Contains(answer.unknownAddressCodes(), answer.Code)
}

func (DirectoryAnswer) unknownAddressCodes() []string {
	return []string{"not_found", "domain_not_hosted"}
}
