package main

import (
	"github.com/pulumi/pulumi-aws/sdk/v7/go/aws/ec2"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type KeyPairResult struct {
	KeyName pulumi.StringOutput
}

func NewKeyPair(ctx *pulumi.Context, name string, publicKey string) (*KeyPairResult, error) {
	kp, err := ec2.NewKeyPair(ctx, name+"-keypair", &ec2.KeyPairArgs{
		KeyName:   pulumi.String(name + "-keypair"),
		PublicKey: pulumi.String(publicKey),
		Tags:      pulumi.StringMap{"Name": pulumi.String(name + "-keypair")},
	})
	if err != nil {
		return nil, err
	}

	return &KeyPairResult{
		KeyName: kp.KeyName,
	}, nil
}
