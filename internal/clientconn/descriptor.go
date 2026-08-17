package clientconn

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	Protocol                = "moda.client-mcp-connection"
	ProtocolVersion         = 1
	TransportStreamableHTTP = "streamable-http"
	maxDescriptorSize       = 64 * 1024
)

type Descriptor struct {
	Protocol         string        `json:"protocol"`
	ProtocolVersion  int           `json:"protocolVersion"`
	Transport        string        `json:"transport"`
	Endpoint         string        `json:"endpoint"`
	LocalAccessToken string        `json:"localAccessToken"`
	RootCAPath       string        `json:"rootCaPath"`
	ExpiresAt        time.Time     `json:"expiresAt"`
	Client           ClientProcess `json:"client"`
}

type ClientProcess struct {
	PID       int       `json:"pid"`
	StartedAt time.Time `json:"startedAt"`
}

func Read(filePath string) (Descriptor, error) {
	return readAt(filePath, time.Now())
}

func readAt(filePath string, now time.Time) (Descriptor, error) {
	info, err := os.Lstat(filePath)
	if err != nil {
		return Descriptor{}, fmt.Errorf("read Moda client connection descriptor %q: %w", filePath, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return Descriptor{}, fmt.Errorf("Moda client connection descriptor %q must not be a symbolic link", filePath)
	}
	if !info.Mode().IsRegular() {
		return Descriptor{}, fmt.Errorf("Moda client connection descriptor %q must be a regular file", filePath)
	}
	if err := validatePrivateFile(info); err != nil {
		return Descriptor{}, fmt.Errorf("Moda client connection descriptor %q: %w", filePath, err)
	}

	file, err := os.Open(filePath)
	if err != nil {
		return Descriptor{}, fmt.Errorf("open Moda client connection descriptor %q: %w", filePath, err)
	}
	defer file.Close()
	openedInfo, err := file.Stat()
	if err != nil {
		return Descriptor{}, fmt.Errorf("inspect open Moda client connection descriptor %q: %w", filePath, err)
	}
	if !os.SameFile(info, openedInfo) {
		return Descriptor{}, fmt.Errorf("Moda client connection descriptor %q changed while it was being opened", filePath)
	}

	limited := io.LimitReader(file, maxDescriptorSize+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return Descriptor{}, fmt.Errorf("read Moda client connection descriptor %q: %w", filePath, err)
	}
	if len(data) > maxDescriptorSize {
		return Descriptor{}, fmt.Errorf("Moda client connection descriptor %q exceeds %d bytes", filePath, maxDescriptorSize)
	}

	var descriptor Descriptor
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&descriptor); err != nil {
		return Descriptor{}, fmt.Errorf("decode Moda client connection descriptor %q: %w", filePath, err)
	}
	if err := ensureJSONEnd(decoder); err != nil {
		return Descriptor{}, fmt.Errorf("decode Moda client connection descriptor %q: %w", filePath, err)
	}
	if err := descriptor.validate(now); err != nil {
		return Descriptor{}, fmt.Errorf("invalid Moda client connection descriptor %q: %w", filePath, err)
	}
	return descriptor, nil
}

func ensureJSONEnd(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); errors.Is(err, io.EOF) {
		return nil
	} else if err != nil {
		return err
	}
	return errors.New("descriptor contains more than one JSON value")
}

func (descriptor Descriptor) validate(now time.Time) error {
	if descriptor.Protocol != Protocol {
		return fmt.Errorf("protocol must be %q", Protocol)
	}
	if descriptor.ProtocolVersion != ProtocolVersion {
		return fmt.Errorf("protocolVersion %d is unsupported; expected %d", descriptor.ProtocolVersion, ProtocolVersion)
	}
	if descriptor.Transport != TransportStreamableHTTP {
		return fmt.Errorf("transport must be %q", TransportStreamableHTTP)
	}
	if err := validateEndpoint(descriptor.Endpoint); err != nil {
		return err
	}
	if strings.TrimSpace(descriptor.LocalAccessToken) == "" {
		return errors.New("localAccessToken must not be empty")
	}
	if !filepath.IsAbs(descriptor.RootCAPath) {
		return errors.New("rootCaPath must be absolute")
	}
	rootCAInfo, err := os.Stat(descriptor.RootCAPath)
	if err != nil {
		return fmt.Errorf("rootCaPath is not readable: %w", err)
	}
	if !rootCAInfo.Mode().IsRegular() {
		return errors.New("rootCaPath must identify a regular file")
	}
	if descriptor.ExpiresAt.IsZero() || !descriptor.ExpiresAt.After(now) {
		return errors.New("descriptor is expired")
	}
	if descriptor.Client.PID <= 0 || descriptor.Client.StartedAt.IsZero() || descriptor.Client.StartedAt.After(now) {
		return errors.New("client process information is invalid")
	}
	if !descriptor.ExpiresAt.After(descriptor.Client.StartedAt) {
		return errors.New("expiresAt must be after the client process startedAt")
	}
	if !processAlive(descriptor.Client.PID) {
		return fmt.Errorf("client process %d is not running", descriptor.Client.PID)
	}
	return nil
}

func validateEndpoint(rawEndpoint string) error {
	endpoint, err := url.Parse(rawEndpoint)
	if err != nil {
		return fmt.Errorf("endpoint is invalid: %w", err)
	}
	hostname := strings.ToLower(endpoint.Hostname())
	isLoopback := hostname == "localhost" || hostname == "127.0.0.1" || hostname == "::1"
	if endpoint.Scheme != "https" || !isLoopback || endpoint.Port() == "" {
		return errors.New("endpoint must be a loopback HTTPS URL with an explicit port")
	}
	if endpoint.Path != "/agent-runtime/v1/mcp/client" || endpoint.RawQuery != "" || endpoint.Fragment != "" || endpoint.User != nil {
		return errors.New("endpoint must use the exact /agent-runtime/v1/mcp/client path without credentials, query, or fragment")
	}
	return nil
}
