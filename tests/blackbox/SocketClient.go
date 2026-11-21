//go:build blackbox

package blackbox

import (
	"bufio"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"testing"
	"time"
)

type SocketClient struct {
	connection net.Conn
	reader     *bufio.Reader
	closeCode  int
}

type SocketEvent struct {
	Type           string `json:"type"`
	ConversationId string `json:"conversationId"`
	MessageId      string `json:"messageId"`
	MemberId       string `json:"memberId"`
	State          string `json:"state"`
}

func DialSocket(t *testing.T, baseUrl string, path string) *SocketClient {
	t.Helper()
	address, err := url.Parse(baseUrl)
	if err != nil {
		t.Fatal(err)
	}
	connection := SocketClient{}.dial(t, address)
	client := &SocketClient{connection: connection, reader: bufio.NewReader(connection)}
	client.upgrade(t, address.Host, path)
	t.Cleanup(client.Close)
	return client
}

func (SocketClient) dial(t *testing.T, address *url.URL) net.Conn {
	t.Helper()
	connection, err := SocketClient{}.open(address)
	if err != nil {
		t.Fatalf("could not reach %s: %v", address.Host, err)
	}
	return connection
}

func (SocketClient) open(address *url.URL) (net.Conn, error) {
	host := SocketClient{}.hostWithPort(address)
	if address.Scheme == "https" {
		return tls.Dial("tcp", host, &tls.Config{ServerName: address.Hostname()})
	}
	return net.DialTimeout("tcp", host, 10*time.Second)
}

func (SocketClient) hostWithPort(address *url.URL) string {
	if address.Port() != "" {
		return address.Host
	}
	return net.JoinHostPort(address.Hostname(), SocketClient{}.defaultPort(address.Scheme))
}

func (SocketClient) defaultPort(scheme string) string {
	if scheme == "https" {
		return "443"
	}
	return "80"
}

func (client *SocketClient) upgrade(t *testing.T, host string, path string) {
	t.Helper()
	key := base64.StdEncoding.EncodeToString(client.randomBytes(t, 16))
	request := fmt.Sprintf("GET %s HTTP/1.1\r\nHost: %s\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: %s\r\nSec-WebSocket-Version: 13\r\n\r\n", path, host, key)
	if _, err := client.connection.Write([]byte(request)); err != nil {
		t.Fatal(err)
	}
	response, err := http.ReadResponse(client.reader, nil)
	if err != nil {
		t.Fatalf("no answer to the socket upgrade: %v", err)
	}
	client.expectSwitch(t, response)
}

func (client *SocketClient) expectSwitch(t *testing.T, response *http.Response) {
	t.Helper()
	if response.StatusCode != http.StatusSwitchingProtocols {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("want 101 for the socket upgrade, got %d: %s", response.StatusCode, body)
	}
}

func (client *SocketClient) SendJson(t *testing.T, value any) {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	client.writeFrame(t, client.textOpcode(), payload)
}

func (client *SocketClient) NextEvent(t *testing.T, within time.Duration) (SocketEvent, bool) {
	t.Helper()
	client.connection.SetReadDeadline(time.Now().Add(within))
	return client.nextText(t)
}

func (client *SocketClient) CloseCode() int {
	return client.closeCode
}

func (client *SocketClient) Close() {
	client.connection.Close()
}

func (client *SocketClient) nextText(t *testing.T) (SocketEvent, bool) {
	t.Helper()
	opcode, payload := client.readFrame(t)
	switch opcode {
	case client.textOpcode():
		return client.decode(t, payload), true
	case client.closeOpcode():
		client.closeCode = client.codeOf(payload)
		return SocketEvent{}, false
	case client.pingOpcode():
		client.writeFrame(t, client.pongOpcode(), payload)
	}
	return client.nextText(t)
}

func (client *SocketClient) decode(t *testing.T, payload []byte) SocketEvent {
	t.Helper()
	var event SocketEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		t.Fatalf("a socket frame is not JSON: %s", payload)
	}
	return event
}

func (client *SocketClient) codeOf(payload []byte) int {
	if len(payload) < 2 {
		return 0
	}
	return int(binary.BigEndian.Uint16(payload[:2]))
}

func (client *SocketClient) readFrame(t *testing.T) (byte, []byte) {
	t.Helper()
	header := client.readBytes(t, 2)
	length := client.payloadLength(t, uint64(header[1]&client.lengthMask()))
	return header[0] & client.opcodeMask(), client.readBytes(t, int(length))
}

func (client *SocketClient) payloadLength(t *testing.T, short uint64) uint64 {
	t.Helper()
	switch short {
	case client.twoByteLength():
		return uint64(binary.BigEndian.Uint16(client.readBytes(t, 2)))
	case client.eightByteLength():
		return binary.BigEndian.Uint64(client.readBytes(t, 8))
	}
	return short
}

func (client *SocketClient) readBytes(t *testing.T, size int) []byte {
	t.Helper()
	buffer := make([]byte, size)
	if _, err := io.ReadFull(client.reader, buffer); err != nil {
		t.Fatalf("the socket ended while reading: %v", err)
	}
	return buffer
}

func (client *SocketClient) writeFrame(t *testing.T, opcode byte, payload []byte) {
	t.Helper()
	mask := client.randomBytes(t, 4)
	frame := append([]byte{client.finalBit() | opcode}, client.lengthBytes(len(payload))...)
	frame = append(frame, mask...)
	frame = append(frame, client.masked(payload, mask)...)
	if _, err := client.connection.Write(frame); err != nil {
		t.Fatal(err)
	}
}

func (client *SocketClient) lengthBytes(size int) []byte {
	switch {
	case size < int(client.twoByteLength()):
		return []byte{client.maskBit() | byte(size)}
	case size <= 0xffff:
		return binary.BigEndian.AppendUint16([]byte{client.maskBit() | byte(client.twoByteLength())}, uint16(size))
	}
	return binary.BigEndian.AppendUint64([]byte{client.maskBit() | byte(client.eightByteLength())}, uint64(size))
}

func (client *SocketClient) masked(payload []byte, mask []byte) []byte {
	result := make([]byte, len(payload))
	for index, value := range payload {
		result[index] = value ^ mask[index%len(mask)]
	}
	return result
}

func (client *SocketClient) randomBytes(t *testing.T, size int) []byte {
	t.Helper()
	raw := make([]byte, size)
	if _, err := rand.Read(raw); err != nil {
		t.Fatal(err)
	}
	return raw
}

func (SocketClient) textOpcode() byte { return 0x1 }

func (SocketClient) closeOpcode() byte { return 0x8 }

func (SocketClient) pingOpcode() byte { return 0x9 }

func (SocketClient) pongOpcode() byte { return 0xa }

func (SocketClient) opcodeMask() byte { return 0x0f }

func (SocketClient) finalBit() byte { return 0x80 }

func (SocketClient) maskBit() byte { return 0x80 }

func (SocketClient) lengthMask() byte { return 0x7f }

func (SocketClient) twoByteLength() uint64 { return 126 }

func (SocketClient) eightByteLength() uint64 { return 127 }
