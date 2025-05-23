module github.com/envoyproxy/go-control-plane/envoy

go 1.22

// Used to resolve import issues related to go-control-plane package split (https://github.com/envoyproxy/go-control-plane/issues/1074)
replace github.com/envoyproxy/go-control-plane@v0.13.4 => ../

require (
	github.com/cncf/xds/go v0.0.0-20240905190251-b4127c9b8d78
	github.com/envoyproxy/go-control-plane v0.13.4
	github.com/envoyproxy/protoc-gen-validate v1.2.1
	github.com/planetscale/vtprotobuf v0.6.1-0.20240319094008-0393e58bdf10
	github.com/prometheus/client_model v0.6.1
	go.opentelemetry.io/proto/otlp v1.0.0
	google.golang.org/genproto/googleapis/api v0.0.0-20241202173237-19429a94021a
	google.golang.org/genproto/googleapis/rpc v0.0.0-20241202173237-19429a94021a
	google.golang.org/grpc v1.70.0
	google.golang.org/protobuf v1.36.4
)

require (
	cel.dev/expr v0.19.0 // indirect
	golang.org/x/net v0.34.0 // indirect
	golang.org/x/sys v0.29.0 // indirect
	golang.org/x/text v0.21.0 // indirect
)
