package main

import (
	"context"
	"net"
	"strings"
)

type DkimResolver struct {
	Records map[string]string
}

func (resolver DkimResolver) LookupTXT(_ context.Context, name string) ([]string, error) {
	record, found := resolver.Records[strings.ToLower(strings.TrimSuffix(name, "."))]
	return resolver.answer(name, record, found)
}

func (resolver DkimResolver) LookupIP(_ context.Context, host string) ([]net.IP, error) {
	return nil, resolver.notFound(host)
}

func (resolver DkimResolver) LookupMX(_ context.Context, name string) ([]*net.MX, error) {
	return nil, resolver.notFound(name)
}

func (resolver DkimResolver) answer(name string, record string, found bool) ([]string, error) {
	if !found {
		return nil, resolver.notFound(name)
	}
	return []string{record}, nil
}

func (DkimResolver) notFound(name string) error {
	return &net.DNSError{Err: "no such host", Name: name, IsNotFound: true}
}
