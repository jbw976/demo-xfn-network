package main

import (
	"context"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/crossplane/crossplane-runtime/pkg/logging"
	fnv1 "github.com/crossplane/function-sdk-go/proto/v1"
	"github.com/crossplane/function-sdk-go/resource"
)

func TestRunFunction(t *testing.T) {
	type args struct {
		ctx context.Context
		req *fnv1.RunFunctionRequest
	}
	type want struct {
		rsp *fnv1.RunFunctionResponse
		err error
	}

	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"AddOneNetworks": {
			reason: "The Function should add one set of network related resources (1 VPC + 1 InternetGateway) to the desired composed resources",
			args: args{
				req: &fnv1.RunFunctionRequest{
					Observed: &fnv1.State{
						Composite: &fnv1.Resource{
							// MustStructJSON is a handy way to provide mock
							// resources.
							Resource: resource.MustStructJSON(`{
								"apiVersion": "xp-layers.crossplane.io/v1alpha1",
								"kind": "XNetwork",
								"metadata": {
									"name": "network-code"
								},
								"spec": {
									"id": "code",
									"count": 1,
									"includeGateway": true,
									"providerConfigName": "default",
									"region": "eu-central-1",
									"compositionSelector": {
										"matchLabels": {
											"layer": "code"
										}
									}
								}
							}`),
						},
					},
				},
			},
			want: want{
				rsp: &fnv1.RunFunctionResponse{
					Meta: &fnv1.ResponseMeta{Ttl: durationpb.New(60 * time.Second)},
					Desired: &fnv1.State{
						Resources: map[string]*fnv1.Resource{
							"vpc-code-0": {Resource: resource.MustStructJSON(`{
								"apiVersion": "ec2.aws.upbound.io/v1beta1",
								"kind": "VPC",
								"metadata": {
									"labels": {
										"networks.meta.fn.crossplane.io/network-id": "code",
										"networks.meta.fn.crossplane.io/vpc-id": "vpc-code-0"
									},
									"name": "vpc-code-0"
								},
								"spec": {
									"forProvider": {
										"cidrBlock": "192.168.0.0/16",
										"enableDnsHostnames": true,
										"enableDnsSupport": true,
										"region": "eu-central-1"
									},
									"providerConfigRef": {
										"name": "default"
									}
								},
								"status": {
									"observedGeneration": 0
								}
							}`)},
							"gateway-code-0": {Resource: resource.MustStructJSON(`{
								"apiVersion": "ec2.aws.upbound.io/v1beta1",
								"kind": "InternetGateway",
								"metadata": {
									"labels": {
										"networks.meta.fn.crossplane.io/network-id": "code"
									},
									"name": "gateway-code-0"
								},
								"spec": {
									"forProvider": {
										"region": "eu-central-1",
										"vpcIdSelector": {
											"matchControllerRef": true,
											"matchLabels": {
												"networks.meta.fn.crossplane.io/vpc-id": "vpc-code-0"
											}
										}
									},
									"providerConfigRef": {
										"name": "default"
									}
								},
								"status": {
									"observedGeneration": 0
								}
							}`)},
						},
					},
				},
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			f := &Function{log: logging.NewNopLogger()}
			rsp, err := f.RunFunction(tc.args.ctx, tc.args.req)

			if diff := cmp.Diff(tc.want.rsp, rsp, protocmp.Transform()); diff != "" {
				t.Errorf("%s\nf.RunFunction(...): -want rsp, +got rsp:\n%s", tc.reason, diff)
			}

			if diff := cmp.Diff(tc.want.err, err, cmpopts.EquateErrors()); diff != "" {
				t.Errorf("%s\nf.RunFunction(...): -want err, +got err:\n%s", tc.reason, diff)
			}
		})
	}
}
