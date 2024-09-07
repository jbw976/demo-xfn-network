package main

import (
	"context"
	"fmt"

	v1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/logging"
	fnv1 "github.com/crossplane/function-sdk-go/proto/v1"
	"github.com/crossplane/function-sdk-go/request"
	"github.com/crossplane/function-sdk-go/resource"
	"github.com/crossplane/function-sdk-go/resource/composed"
	"github.com/crossplane/function-sdk-go/response"
	"github.com/pkg/errors"

	awsv1beta1 "github.com/upbound/provider-aws/apis/ec2/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

type Function struct {
	fnv1.UnimplementedFunctionRunnerServiceServer

	log logging.Logger
}

// RunFunction implements our custom full code function logic. It will create a
// variable number of VPCs and conditionally create InternetGateways for each
// VPC.
func (f *Function) RunFunction(_ context.Context, req *fnv1.RunFunctionRequest) (*fnv1.RunFunctionResponse, error) {
	f.log.Info("Running function", "tag", req.GetMeta().GetTag())

	rsp := response.To(req, response.DefaultTTL)

	// get the observed XR so we can read all the specified config from it
	oxr, err := request.GetObservedCompositeResource(req)
	if err != nil {
		response.Fatal(rsp, errors.Wrapf(err, "cannot get desired XR"))
		return rsp, nil
	}

	// retrieve all the specified config from the XR
	id, _ := oxr.Resource.GetString("spec.id")
	count, _ := oxr.Resource.GetInteger("spec.count")
	includeGateway, _ := oxr.Resource.GetBool("spec.includeGateway")
	region, _ := oxr.Resource.GetString("spec.region")
	if region == "" {
		region = "eu-central-1"
	}
	providerConfigName, _ := oxr.Resource.GetString("spec.providerConfigName")
	if providerConfigName == "" {
		providerConfigName = "default"
	}

	// get a reference to the desired composed resources, so we can add our
	// desired VPCs and InternetGateways to this list
	desired, err := request.GetDesiredComposedResources(req)
	if err != nil {
		response.Fatal(rsp, errors.Wrapf(err, "cannot get desired resources from %T", req))
		return rsp, nil
	}

	// Add the AWS EC2 v1beta1 types (including VPC and InternetGateway) to the
	// composed resource scheme. composed.From uses this to automatically set
	// apiVersion and kind.
	_ = awsv1beta1.AddToScheme(composed.Scheme)

	// Iterate over the desired count of network resources, creating 1 resource per iteration
	for i := range count {
		// configure the VPC resource
		vpcName := fmt.Sprintf("vpc-%s-%d", id, i)
		vpc := &awsv1beta1.VPC{
			ObjectMeta: metav1.ObjectMeta{
				Name: vpcName,
				Labels: map[string]string{
					"networks.meta.fn.crossplane.io/network-id": id,
					"networks.meta.fn.crossplane.io/vpc-id":     vpcName,
				},
			},
			Spec: awsv1beta1.VPCSpec{
				ForProvider: awsv1beta1.VPCParameters_2{
					Region:             ptr.To(region),
					CidrBlock:          ptr.To("192.168.0.0/16"),
					EnableDNSSupport:   ptr.To(true),
					EnableDNSHostnames: ptr.To(true),
				},
				ResourceSpec: v1.ResourceSpec{
					ProviderConfigReference: &v1.Reference{Name: providerConfigName},
				},
			},
		}

		// add the VPC resource to the desired composed resources
		dcVPC, err := composed.From(vpc)
		if err != nil {
			response.Fatal(rsp, errors.Wrapf(err, "cannot convert %T to %T", vpc, &composed.Unstructured{}))
			return rsp, nil
		}
		desired[resource.Name(vpcName)] = &resource.DesiredComposed{Resource: dcVPC}

		if includeGateway {
			// the user wants an InternetGateway to be created also, configure one now
			gatewayName := fmt.Sprintf("gateway-%s-%d", id, i)
			gateway := &awsv1beta1.InternetGateway{
				ObjectMeta: metav1.ObjectMeta{
					Name: gatewayName,
					Labels: map[string]string{
						"networks.meta.fn.crossplane.io/network-id": id,
					},
				},
				Spec: awsv1beta1.InternetGatewaySpec{
					ForProvider: awsv1beta1.InternetGatewayParameters_2{
						Region: ptr.To(region),
						VPCIDSelector: &v1.Selector{
							MatchControllerRef: ptr.To(true),
							MatchLabels: map[string]string{
								"networks.meta.fn.crossplane.io/vpc-id": vpcName,
							},
						},
					},
					ResourceSpec: v1.ResourceSpec{
						ProviderConfigReference: &v1.Reference{Name: providerConfigName},
					},
				},
			}

			// add the InternetGateway resource to the desired composed resources
			dcGateway, err := composed.From(gateway)
			if err != nil {
				response.Fatal(rsp, errors.Wrapf(err, "cannot convert %T to %T", gateway, &composed.Unstructured{}))
				return rsp, nil
			}
			desired[resource.Name(gatewayName)] = &resource.DesiredComposed{Resource: dcGateway}
		}

	}

	// set the desired composed resources back on the response
	if err := response.SetDesiredComposedResources(rsp, desired); err != nil {
		response.Fatal(rsp, errors.Wrapf(err, "cannot set desired composed resources in %T", rsp))
		return rsp, nil
	}

	f.log.Info("Function ran OK", "id", id, "count", count, "includeGateway", includeGateway, "region", region, "providerConfigName", providerConfigName)
	return rsp, nil
}
