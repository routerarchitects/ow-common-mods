package common

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/client"
	"github.com/routerarchitects/ow-common-mods/servicediscovery"
	"github.com/routerarchitects/ra-common-mods/apperror"
)

type ServiceInstance struct {
	Key             string
	PrivateEndPoint string
}

type ServiceResolver interface {
	Resolve(serviceName string) (*ServiceInstance, error)
}

type Response interface {
	StatusCode() int
	Body() []byte
	Close()
}

type Requester interface {
	Send(ctx context.Context, method, url string, headers map[string]string, body []byte) (Response, error)
}

type discoveryResolver struct {
	discovery *servicediscovery.Discovery
}

func NewDiscoveryResolver(discovery *servicediscovery.Discovery) ServiceResolver {
	return &discoveryResolver{discovery: discovery}
}

func (r *discoveryResolver) Resolve(serviceName string) (*ServiceInstance, error) {
	if r.discovery == nil {
		return nil, apperror.New(apperror.CodeInternal, "service discovery is not configured")
	}

	instance := r.discovery.Store().GetServiceInstances(serviceName)
	if instance == nil {
		return nil, apperror.New(apperror.CodeNotFound, "service instance not found")
	}

	return &ServiceInstance{
		Key:             instance.Key,
		PrivateEndPoint: instance.PrivateEndPoint,
	}, nil
}

type FiberRequester struct {
	client *client.Client
}

func NewFiberRequester(tlsRootCA string) (*FiberRequester, error) {
	fiberClient := client.New()

	if strings.TrimSpace(tlsRootCA) != "" {
		pemBytes, err := os.ReadFile(tlsRootCA)
		if err != nil {
			return nil, apperror.Wrap(apperror.CodeInternal, fmt.Sprintf("read TLS root CA %q", tlsRootCA), err)
		}

		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pemBytes) {
			return nil, apperror.New(apperror.CodeInternal, fmt.Sprintf("parse TLS root CA %q: invalid PEM", tlsRootCA))
		}
		fiberClient.TLSConfig().RootCAs = pool
	}

	return &FiberRequester{client: fiberClient}, nil
}

func (r *FiberRequester) Send(ctx context.Context, method, url string, headers map[string]string, body []byte) (Response, error) {
	req := r.client.R().
		SetContext(ctx).
		SetMethod(method).
		SetURL(url)

	for k, v := range headers {
		req = req.SetHeader(k, v)
	}
	if len(body) > 0 {
		req = req.SetRawBody(body)
	}

	resp, err := req.Send()
	if err != nil {
		return nil, err
	}
	return resp, nil
}

type ServiceRPCBase struct {
	resolver     ServiceResolver
	requester    Requester
	internalName string
	logger       *slog.Logger
}

// NewServiceRPCBase builds the default RPC base using service discovery resolver
// and Fiber requester.
//
// Callers are expected to pass a context with timeout/deadline to every client
// API method; this module does not impose an internal request timeout.
func NewServiceRPCBase(
	discovery *servicediscovery.Discovery,
	tlsRootCA string,
	internalName string,
	logger *slog.Logger,
) (*ServiceRPCBase, error) {
	if discovery == nil {
		return nil, apperror.New(apperror.CodeInvalidInput, "discovery is required.")
	}
	requester, err := NewFiberRequester(tlsRootCA)
	if err != nil {
		return nil, err
	}
	return NewServiceRPCBaseWithDeps(NewDiscoveryResolver(discovery), requester, internalName, logger)
}

func NewServiceRPCBaseWithDeps(
	resolver ServiceResolver,
	requester Requester,
	internalName string,
	logger *slog.Logger,
) (*ServiceRPCBase, error) {
	if resolver == nil {
		return nil, apperror.New(apperror.CodeInvalidInput, "service resolver is required")
	}
	if requester == nil {
		return nil, apperror.New(apperror.CodeInvalidInput, "requester is required")
	}
	if strings.TrimSpace(internalName) == "" {
		return nil, apperror.New(apperror.CodeInvalidInput, "internalName is required")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &ServiceRPCBase{
		resolver:     resolver,
		requester:    requester,
		internalName: internalName,
		logger:       logger,
	}, nil
}

func (s *ServiceRPCBase) Logger() *slog.Logger {
	if s == nil || s.logger == nil {
		return slog.Default()
	}
	return s.logger
}

// Send dispatches a request to the discovered service endpoint.
//
// The provided ctx must include timeout/deadline from the caller. This function
// uses caller context as the single source of cancellation/timeout behavior.
func (s *ServiceRPCBase) Send(ctx context.Context, method string, endpoint string, body io.Reader, serviceName string) (Response, error) {
	if s == nil {
		return nil, apperror.New(apperror.CodeInternal, "service rpc base is nil")
	}
	if ctx == nil {
		return nil, apperror.New(apperror.CodeInvalidInput, "context is required")
	}
	if strings.TrimSpace(method) == "" {
		return nil, apperror.New(apperror.CodeInvalidInput, "http method is required")
	}
	if strings.TrimSpace(endpoint) == "" {
		return nil, apperror.New(apperror.CodeInvalidInput, "endpoint is required")
	}
	if strings.TrimSpace(serviceName) == "" {
		return nil, apperror.New(apperror.CodeInvalidInput, "serviceName is required")
	}

	service, err := s.resolveService(serviceName)
	if err != nil {
		return nil, err
	}

	var rawBody []byte

	if body != nil {
		var readErr error
		rawBody, readErr = io.ReadAll(body)
		if readErr != nil {
			return nil, apperror.Wrap(apperror.CodeInternal, "failed to read request body", readErr)
		}
	}

	headers := map[string]string{
		"X-API-KEY":       service.Key,
		"X-INTERNAL-NAME": s.internalName,
	}
	if len(rawBody) > 0 {
		headers[fiber.HeaderContentType] = fiber.MIMEApplicationJSON
	}

	url := strings.TrimSuffix(service.PrivateEndPoint, "/") + endpoint
	resp, sendErr := s.requester.Send(ctx, method, url, headers, rawBody)
	if sendErr != nil {
		if errors.Is(sendErr, context.Canceled) || errors.Is(sendErr, context.DeadlineExceeded) {
			return nil, apperror.WrapWithMeta(apperror.CodeTimeout, "request timeout", sendErr, map[string]any{
				"endpoint":    endpoint,
				"serviceName": serviceName,
			})
		}
		var appErr *apperror.Error
		if errors.As(sendErr, &appErr) {
			return nil, sendErr
		}
		return nil, apperror.Wrap(apperror.CodeInternal, fmt.Sprintf("%s request failed", serviceName), sendErr)
	}

	return resp, nil
}

func (s *ServiceRPCBase) resolveService(serviceName string) (*ServiceInstance, error) {
	if s.resolver == nil {
		return nil, apperror.New(apperror.CodeInternal, "service resolver is not configured")
	}

	return s.resolver.Resolve(serviceName)
}
