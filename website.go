package main

import (
	"fmt"

	"github.com/pulumi/pulumi-aws/sdk/v7/go/aws/ec2"
	"github.com/pulumi/pulumi-aws/sdk/v7/go/aws/iam"
	"github.com/pulumi/pulumi-aws/sdk/v7/go/aws/s3"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type AwsS3Website struct {
	pulumi.ResourceState
	Url      pulumi.StringOutput // EC2 public IP — for SSH management only.
	BucketID pulumi.IDOutput     // the S3 bucket ID.
}

type AwsS3WebsiteArgs struct {
	Files             []string // a list of files to serve.
	ZeroTierNetworkID string   // ZeroTier network ID to join, e.g. "166359304e80c3b5".
}

func NewAwsS3Website(ctx *pulumi.Context, name string, args AwsS3WebsiteArgs, opts ...pulumi.ResourceOption) (*AwsS3Website, error) {
	self := &AwsS3Website{}
	err := ctx.RegisterComponentResource("quickstart:index:AwsS3Website", name, self, opts...)
	if err != nil {
		return nil, err
	}

	// Private S3 bucket — stores website files, not publicly accessible.
	bucket, err := s3.NewBucket(ctx, "my-bucket", nil, pulumi.Parent(self))
	if err != nil {
		return nil, err
	}

	// Upload website files to S3 (private, EC2 fetches them at boot).
	for _, file := range args.Files {
		_, err = s3.NewBucketObject(ctx, file, &s3.BucketObjectArgs{
			Bucket:      bucket.ID(),
			Source:      pulumi.NewFileAsset(file),
			ContentType: pulumi.String("text/html"),
		}, pulumi.Parent(self))
		if err != nil {
			return nil, err
		}
	}

	// IAM role so the EC2 instance can read files from S3.
	role, err := iam.NewRole(ctx, "ec2-role", &iam.RoleArgs{
		AssumeRolePolicy: pulumi.String(`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"ec2.amazonaws.com"},"Action":"sts:AssumeRole"}]}`),
	}, pulumi.Parent(self))
	if err != nil {
		return nil, err
	}

	_, err = iam.NewRolePolicy(ctx, "ec2-s3-policy", &iam.RolePolicyArgs{
		Role: role.Name,
		Policy: bucket.Arn.ApplyT(func(arn string) (string, error) {
			return fmt.Sprintf(`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":["s3:GetObject","s3:ListBucket"],"Resource":["%s","%s/*"]}]}`, arn, arn), nil
		}).(pulumi.StringOutput),
	}, pulumi.Parent(self))
	if err != nil {
		return nil, err
	}

	instanceProfile, err := iam.NewInstanceProfile(ctx, "ec2-profile", &iam.InstanceProfileArgs{
		Role: role.Name,
	}, pulumi.Parent(self))
	if err != nil {
		return nil, err
	}

	// Security group: ZeroTier (UDP 9993) and SSH (TCP 22) inbound only.
	// Port 80 is intentionally not opened — HTTP is tunnelled inside ZeroTier.
	sg, err := ec2.NewSecurityGroup(ctx, "zerotier-sg", &ec2.SecurityGroupArgs{
		Description: pulumi.String("ZeroTier web server"),
		Ingress: ec2.SecurityGroupIngressArray{
			ec2.SecurityGroupIngressArgs{
				Protocol:    pulumi.String("udp"),
				FromPort:    pulumi.Int(9993),
				ToPort:      pulumi.Int(9993),
				CidrBlocks:  pulumi.StringArray{pulumi.String("0.0.0.0/0")},
				Description: pulumi.String("ZeroTier"),
			},
			ec2.SecurityGroupIngressArgs{
				Protocol:    pulumi.String("tcp"),
				FromPort:    pulumi.Int(22),
				ToPort:      pulumi.Int(22),
				CidrBlocks:  pulumi.StringArray{pulumi.String("0.0.0.0/0")},
				Description: pulumi.String("SSH"),
			},
		},
		Egress: ec2.SecurityGroupEgressArray{
			ec2.SecurityGroupEgressArgs{
				Protocol:   pulumi.String("-1"),
				FromPort:   pulumi.Int(0),
				ToPort:     pulumi.Int(0),
				CidrBlocks: pulumi.StringArray{pulumi.String("0.0.0.0/0")},
			},
		},
	}, pulumi.Parent(self))
	if err != nil {
		return nil, err
	}

	// Look up the latest Ubuntu 24.04 LTS AMI (snapd pre-installed, required for ZeroTier snap).
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

	// User data: install nginx + ZeroTier via snap, join the network, pull files from S3.
	networkID := args.ZeroTierNetworkID
	userData := bucket.ID().ApplyT(func(bucketName string) (string, error) {
		return fmt.Sprintf(`#!/bin/bash
set -e
apt-get update -y
apt-get install -y nginx
systemctl enable nginx
snap install zerotier
snap install aws-cli --classic
sleep 5
zerotier join %s
aws s3 cp s3://%s/index.html /var/www/html/index.html
systemctl start nginx
`, networkID, bucketName), nil
	}).(pulumi.StringOutput)

	instance, err := ec2.NewInstance(ctx, "zerotier-server", &ec2.InstanceArgs{
		Ami:                 pulumi.String(ami.Id),
		InstanceType:        pulumi.String("t3.micro"),
		IamInstanceProfile:  instanceProfile.Name,
		VpcSecurityGroupIds: pulumi.StringArray{sg.ID()},
		UserData:            userData,
		Tags:                pulumi.StringMap{"Name": pulumi.String("zerotier-web-server")},
	}, pulumi.Parent(self))
	if err != nil {
		return nil, err
	}

	// Export the EC2 public IP for SSH access.
	// The actual website URL is http://<zerotier-ip>/ — visible in ZeroTier Central after authorising the device.
	self.Url = instance.PublicIp.ApplyT(func(ip string) (string, error) {
		return fmt.Sprintf("ssh ubuntu@%s", ip), nil
	}).(pulumi.StringOutput)

	self.BucketID = bucket.ID()

	ctx.RegisterResourceOutputs(self, pulumi.Map{"url": self.Url, "bucketId": self.BucketID})
	return self, nil
}
