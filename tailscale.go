package main

import (
	"github.com/pulumi/pulumi-aws/sdk/v7/go/aws/ec2"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type TailscaleRouterArgs struct {
	SubnetID        pulumi.IDOutput
	SecurityGroupID pulumi.IDOutput
	AuthKey         pulumi.StringInput
	VpcCidr         string
	KeyName         pulumi.StringOutput
}

type TailscaleRouterResult struct {
	PublicIP pulumi.StringOutput
}

func NewTailscaleRouter(ctx *pulumi.Context, name string, args TailscaleRouterArgs) (*TailscaleRouterResult, error) {
	ami, err := ec2.LookupAmi(ctx, &ec2.LookupAmiArgs{
		MostRecent: pulumi.BoolRef(true),
		Owners:     []string{"099720109477"}, // Canonical
		Filters: []ec2.GetAmiFilter{
			{Name: "name", Values: []string{"ubuntu/images/hvm-ssd-gp3/ubuntu-noble-24.04-amd64-server-*"}},
			{Name: "virtualization-type", Values: []string{"hvm"}},
		},
	})
	if err != nil {
		return nil, err
	}

	userData := args.AuthKey.ToStringOutput().ApplyT(func(authKey string) (string, error) {
		return `#!/bin/bash
set -e
curl -fsSL https://tailscale.com/install.sh | sh
echo 'net.ipv4.ip_forward = 1' >> /etc/sysctl.d/99-tailscale.conf
echo 'net.ipv6.conf.all.forwarding = 1' >> /etc/sysctl.d/99-tailscale.conf
sysctl -p /etc/sysctl.d/99-tailscale.conf
tailscale up --authkey=` + authKey + ` --advertise-routes=` + args.VpcCidr + `
`, nil
	}).(pulumi.StringOutput)

	instance, err := ec2.NewInstance(ctx, name+"-tailscale-router", &ec2.InstanceArgs{
		Ami:                      pulumi.String(ami.Id),
		InstanceType:             pulumi.String("t3.nano"),
		SubnetId:                 args.SubnetID,
		VpcSecurityGroupIds:      pulumi.StringArray{args.SecurityGroupID},
		SourceDestCheck:          pulumi.Bool(false),
		AssociatePublicIpAddress: pulumi.Bool(true),
		KeyName:                  args.KeyName,
		UserData:                 userData,
		UserDataReplaceOnChange:  pulumi.Bool(true),
		Tags:                     pulumi.StringMap{"Name": pulumi.String(name + "-tailscale-router")},
	})
	if err != nil {
		return nil, err
	}

	return &TailscaleRouterResult{
		PublicIP: instance.PublicIp,
	}, nil
}
