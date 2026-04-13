package common

import (
	"context"
	"crypto/x509"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/client"
	"github.com/routerarchitects/ow-common-mods/apperrors"
	"github.com/routerarchitects/ow-common-mods/servicediscovery"
)

type ServiceRPCBase struct {
	Discovery    *servicediscovery.Discovery
	Client       *client.Client
	Timeout      time.Duration
	InternalName string
	Logger       *slog.Logger
}

func NewServiceRPCBase(
	discovery *servicediscovery.Discovery,
	tlsRootCA string,
	timeout time.Duration,
	internalName string,
	Logger *slog.Logger,
) *ServiceRPCBase {
	return &ServiceRPCBase{
		Discovery:    discovery,
		Client:       NewFiberClient(timeout, tlsRootCA),
		Timeout:      timeout,
		InternalName: internalName,
		Logger:       Logger,
	}
}

func NewFiberClient(timeout time.Duration, tlsRootCA string) *client.Client {
	fiberClient := client.New()
	fiberClient.SetTimeout(timeout)

	if strings.TrimSpace(tlsRootCA) == "" {
		return fiberClient
	}

	pemBytes, err := os.ReadFile(tlsRootCA)
	if err != nil {
		panic(fmt.Sprintf("read TLS root CA %q: %v", tlsRootCA, err))
	}

	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pemBytes) {
		panic(fmt.Sprintf("parse TLS root CA %q: invalid PEM", tlsRootCA))
	}

	fiberClient.TLSConfig().RootCAs = pool
	return fiberClient
}

func (s *ServiceRPCBase) Send(ctx context.Context, method string, endpoint string, body io.Reader, servicename string) (*client.Response, error) {
	service, err := s.resolveService(servicename)
	if err != nil {
		return nil, err
	}

	reqCtx, cancel := context.WithTimeout(ctx, s.Timeout)
	defer cancel()

	req := s.Client.R().
		SetContext(reqCtx).
		SetTimeout(s.Timeout).
		SetMethod(method).
		SetHeader("X-API-KEY", service.Key).
		SetHeader("X-INTERNAL-NAME", s.InternalName).
		SetURL(strings.TrimSuffix(service.PrivateEndPoint, "/") + endpoint)

	if body != nil {
		rawBody, err := io.ReadAll(body)
		if err != nil {
			return nil, apperrors.Wrap(apperrors.CodeInternal, "failed to read body", err)
		}
		req = req.SetRawBody(rawBody).SetHeader(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
	}

	resp, err := req.Send()
	if err != nil {
		return nil, apperrors.Wrap(apperrors.CodeInternal, "analytics request failed", err)
	}

	return resp, nil
}

func (s *ServiceRPCBase) resolveService(servicename string) (*servicediscovery.Instance, error) {
	if s.Discovery == nil {
		return nil, apperrors.Wrap(apperrors.CodeInternal, "service discovery is not configured", nil)
	}

	services := s.Discovery.Store().GetServiceInstances(servicename)
	if services == nil {
		return nil, apperrors.Wrap(apperrors.CodeNotFound, "Not Found", nil)
	}

	return services, nil
}
