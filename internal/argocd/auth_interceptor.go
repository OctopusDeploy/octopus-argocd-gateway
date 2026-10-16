package argocd

import (
	"context"

	"google.golang.org/grpc"
)

type authInterceptor struct {
	clientConfig Config
	resolver     TokenResolver
}

func newAuthInterceptor(config Config, resolver TokenResolver) *authInterceptor {
	return &authInterceptor{
		clientConfig: config,
		resolver:     resolver,
	}
}

func (a *authInterceptor) Unary() grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		token, err := a.resolver.Resolve(ctx)
		if err != nil {
			return err
		}

		if a.clientConfig.TLS.Plaintext || a.clientConfig.GRPCWeb || a.clientConfig.GRPCWebRootPath != "" {
			opts = append(opts, grpc.PerRPCCredentials(plaintextRpcAuth{bearerToken: token}))
		} else {
			opts = append(opts, grpc.PerRPCCredentials(rpcAuth{bearerToken: token}))
		}

		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

func (a *authInterceptor) Stream() grpc.StreamClientInterceptor {
	return func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
		token, err := a.resolver.Resolve(ctx)
		if err != nil {
			return nil, err
		}

		if a.clientConfig.TLS.Plaintext || a.clientConfig.GRPCWeb || a.clientConfig.GRPCWebRootPath != "" {
			opts = append(opts, grpc.PerRPCCredentials(plaintextRpcAuth{bearerToken: token}))
		} else {
			opts = append(opts, grpc.PerRPCCredentials(rpcAuth{bearerToken: token}))
		}

		return streamer(ctx, desc, cc, method, opts...)
	}
}
