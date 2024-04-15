package grpc

import (
	"context"
	envoyAuth "github.com/envoyproxy/go-control-plane/envoy/service/auth/v3"
	"github.com/gogo/googleapis/google/rpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"net/http"
)

func checkGRPCAuth(ctx context.Context, authClient envoyAuth.AuthorizationClient) error {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return status.Errorf(codes.Unauthenticated, "missing metadata")
	}

	mdHeaders := make(map[string]string)
	for k, v := range md {
		if len(v) > 0 {
			mdHeaders[k] = v[0]
		}
	}

	result, err := authClient.Check(ctx, &envoyAuth.CheckRequest{
		Attributes: &envoyAuth.AttributeContext{
			Request: &envoyAuth.AttributeContext_Request{
				Http: &envoyAuth.AttributeContext_HttpRequest{
					Headers: mdHeaders,
				},
			},
		},
	})
	if err != nil {
		return status.Errorf(codes.Unauthenticated, "authentication failed: %v", err)
	}

	if result.GetStatus() == nil || result.GetStatus().GetCode() != int32(rpc.OK) {
		return status.Errorf(codes.Unauthenticated, http.StatusText(http.StatusUnauthorized))
	}

	return nil
}

func CheckGRPCAuthUnaryInterceptorWrapper(authClient envoyAuth.AuthorizationClient) func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if err := checkGRPCAuth(ctx, authClient); err != nil {
			return nil, err
		}
		return handler(ctx, req)
	}
}

func CheckGRPCAuthStreamInterceptorWrapper(authClient envoyAuth.AuthorizationClient) func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		if err := checkGRPCAuth(ss.Context(), authClient); err != nil {
			return err
		}
		return handler(srv, ss)
	}
}
