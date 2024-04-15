// Code generated by protoc-gen-go-grpc. DO NOT EDIT.
// versions:
// - protoc-gen-go-grpc v1.2.0
// - protoc             v4.25.3
// source: describe.proto

package golang

import (
	context "context"
	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
)

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
// Requires gRPC-Go v1.32.0 or later.
const _ = grpc.SupportPackageIsVersion7

// DescribeServiceClient is the client API for DescribeService service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
type DescribeServiceClient interface {
	DeliverResult(ctx context.Context, in *DeliverResultRequest, opts ...grpc.CallOption) (*ResponseOK, error)
	SetInProgress(ctx context.Context, in *SetInProgressRequest, opts ...grpc.CallOption) (*ResponseOK, error)
}

type describeServiceClient struct {
	cc grpc.ClientConnInterface
}

func NewDescribeServiceClient(cc grpc.ClientConnInterface) DescribeServiceClient {
	return &describeServiceClient{cc}
}

func (c *describeServiceClient) DeliverResult(ctx context.Context, in *DeliverResultRequest, opts ...grpc.CallOption) (*ResponseOK, error) {
	out := new(ResponseOK)
	err := c.cc.Invoke(ctx, "/kaytu.describe.v1.DescribeService/DeliverResult", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *describeServiceClient) SetInProgress(ctx context.Context, in *SetInProgressRequest, opts ...grpc.CallOption) (*ResponseOK, error) {
	out := new(ResponseOK)
	err := c.cc.Invoke(ctx, "/kaytu.describe.v1.DescribeService/SetInProgress", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// DescribeServiceServer is the server API for DescribeService service.
// All implementations must embed UnimplementedDescribeServiceServer
// for forward compatibility
type DescribeServiceServer interface {
	DeliverResult(context.Context, *DeliverResultRequest) (*ResponseOK, error)
	SetInProgress(context.Context, *SetInProgressRequest) (*ResponseOK, error)
	mustEmbedUnimplementedDescribeServiceServer()
}

// UnimplementedDescribeServiceServer must be embedded to have forward compatible implementations.
type UnimplementedDescribeServiceServer struct {
}

func (UnimplementedDescribeServiceServer) DeliverResult(context.Context, *DeliverResultRequest) (*ResponseOK, error) {
	return nil, status.Errorf(codes.Unimplemented, "method DeliverResult not implemented")
}
func (UnimplementedDescribeServiceServer) SetInProgress(context.Context, *SetInProgressRequest) (*ResponseOK, error) {
	return nil, status.Errorf(codes.Unimplemented, "method SetInProgress not implemented")
}
func (UnimplementedDescribeServiceServer) mustEmbedUnimplementedDescribeServiceServer() {}

// UnsafeDescribeServiceServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to DescribeServiceServer will
// result in compilation errors.
type UnsafeDescribeServiceServer interface {
	mustEmbedUnimplementedDescribeServiceServer()
}

func RegisterDescribeServiceServer(s grpc.ServiceRegistrar, srv DescribeServiceServer) {
	s.RegisterService(&DescribeService_ServiceDesc, srv)
}

func _DescribeService_DeliverResult_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(DeliverResultRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(DescribeServiceServer).DeliverResult(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/kaytu.describe.v1.DescribeService/DeliverResult",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(DescribeServiceServer).DeliverResult(ctx, req.(*DeliverResultRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _DescribeService_SetInProgress_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(SetInProgressRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(DescribeServiceServer).SetInProgress(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/kaytu.describe.v1.DescribeService/SetInProgress",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(DescribeServiceServer).SetInProgress(ctx, req.(*SetInProgressRequest))
	}
	return interceptor(ctx, in, info, handler)
}

// DescribeService_ServiceDesc is the grpc.ServiceDesc for DescribeService service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var DescribeService_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "kaytu.describe.v1.DescribeService",
	HandlerType: (*DescribeServiceServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "DeliverResult",
			Handler:    _DescribeService_DeliverResult_Handler,
		},
		{
			MethodName: "SetInProgress",
			Handler:    _DescribeService_SetInProgress_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "describe.proto",
}
