package grpc

import (
	"context"
	"net/http"

	envoyAuth "github.com/envoyproxy/go-control-plane/envoy/service/auth/v3"
	"github.com/gogo/googleapis/google/rpc"
	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func checkGRPCAuth(ctx context.Context, authClient envoyAuth.AuthorizationClient) (context.Context, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ctx, status.Errorf(codes.Unauthenticated, "missing metadata")
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
		return ctx, status.Errorf(codes.Unauthenticated, "authentication failed: %v", err)
	}

	if result.GetStatus() == nil || result.GetStatus().GetCode() != int32(rpc.OK) || result.GetOkResponse() == nil {
		return ctx, status.Errorf(codes.Unauthenticated, http.StatusText(http.StatusUnauthorized))
	}

	for _, header := range result.GetOkResponse().GetHeaders() {
		if header.GetHeader() == nil {
			continue
		}
		md.Append(header.GetHeader().GetKey(), header.GetHeader().GetValue())
	}

	ctx = metadata.NewIncomingContext(ctx, md)
	return ctx, nil
}

func CheckGRPCAuthUnaryInterceptorWrapper(authClient envoyAuth.AuthorizationClient) func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if ctx, err := checkGRPCAuth(ctx, authClient); err != nil {
			return nil, err
		} else {
			return handler(ctx, req)
		}
	}
}

func CheckGRPCAuthStreamInterceptorWrapper(authClient envoyAuth.AuthorizationClient) func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		if ctx, err := checkGRPCAuth(ss.Context(), authClient); err != nil {
			return err
		} else {
			return handler(srv, &grpc_middleware.WrappedServerStream{
				ServerStream:   ss,
				WrappedContext: ctx,
			})
		}
	}
}
