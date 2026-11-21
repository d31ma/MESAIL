package mailin

import (
	"encoding/json"
	"fmt"
	"io"
	"mesail.dev/packages/oidc"
	"net/http"
	"net/url"
	"strings"
)

type Directory struct {
	Client    *http.Client
	BaseUrl   string
	Authorize func(realm string) (string, error)
}

func (Directory) InstallWide() string {
	return oidc.InstallWide{}.DomainId()
}

func (directory Directory) Domain(domainId string) (Domain, bool, error) {
	response, err := directory.send(directory.domainUrl(domainId))
	if err != nil {
		return Domain{}, false, err
	}
	defer response.Body.Close()
	return directory.answer(response)
}

func (directory Directory) KnownAddress(address string) (bool, error) {
	response, err := directory.send(directory.addressUrl(address))
	if err != nil {
		return false, err
	}
	defer response.Body.Close()
	return directory.addressAnswer(response)
}

func (directory Directory) send(target string) (*http.Response, error) {
	request, err := directory.request(target)
	if err != nil {
		return nil, err
	}
	return directory.Client.Do(request)
}

func (directory Directory) request(target string) (*http.Request, error) {
	authorization, err := directory.Authorize(directory.InstallWide())
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequest(http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", authorization)
	return request, nil
}

func (directory Directory) domainUrl(domainId string) string {
	return directory.base() + "/api/v1/directory/domains/" + url.PathEscape(strings.ToLower(domainId))
}

func (directory Directory) addressUrl(address string) string {
	return directory.base() + "/api/v1/directory/addresses/" + url.PathEscape(address)
}

func (directory Directory) base() string {
	return strings.TrimRight(directory.BaseUrl, "/")
}

func (directory Directory) answer(response *http.Response) (Domain, bool, error) {
	switch response.StatusCode {
	case http.StatusOK:
		return directory.decoded(response.Body)
	case http.StatusNotFound:
		return Domain{}, false, nil
	}
	return Domain{}, false, directory.unexpected(response.StatusCode)
}

func (directory Directory) addressAnswer(response *http.Response) (bool, error) {
	switch response.StatusCode {
	case http.StatusOK:
		return true, nil
	case http.StatusNotFound:
		return directory.notFoundAddress(response.Body)
	}
	return false, directory.unexpected(response.StatusCode)
}

func (directory Directory) notFoundAddress(body io.Reader) (bool, error) {
	answer := DirectoryAnswer{}
	json.NewDecoder(io.LimitReader(body, directory.readLimit())).Decode(&answer)
	return false, directory.unknownUnless(answer)
}

func (directory Directory) unknownUnless(answer DirectoryAnswer) error {
	if answer.IsUnknownAddress() {
		return nil
	}
	return fmt.Errorf("the directory answered %d with code %q", http.StatusNotFound, answer.Code)
}

func (Directory) unexpected(status int) error {
	return fmt.Errorf("the directory answered %d", status)
}

func (directory Directory) decoded(body io.Reader) (Domain, bool, error) {
	domain := Domain{}
	err := json.NewDecoder(io.LimitReader(body, directory.readLimit())).Decode(&domain)
	return domain, err == nil, err
}

func (Directory) readLimit() int64 {
	return 1048576
}
