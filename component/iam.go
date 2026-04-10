package component

import (
	"fmt"

	"github.com/pulumi/pulumi-aws/sdk/v7/go/aws/iam"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type IamArgs struct {
	BucketArn pulumi.StringInput
}

type IamResult struct {
	InstanceProfileName pulumi.StringOutput
}

func NewIam(ctx *pulumi.Context, name string, args IamArgs) (*IamResult, error) {
	role, err := iam.NewRole(ctx, name+"-ec2-role", &iam.RoleArgs{
		AssumeRolePolicy: pulumi.String(`{
			"Version": "2012-10-17",
			"Statement": [{
				"Effect": "Allow",
				"Principal": {"Service": "ec2.amazonaws.com"},
				"Action": "sts:AssumeRole"
			}]
		}`),
		Tags: pulumi.StringMap{"Name": pulumi.String(name + "-ec2-role")},
	})
	if err != nil {
		return nil, err
	}

	_, err = iam.NewRolePolicy(ctx, name+"-ec2-s3-policy", &iam.RolePolicyArgs{
		Role: role.Name,
		Policy: args.BucketArn.ToStringOutput().ApplyT(func(arn string) (string, error) {
			return fmt.Sprintf(`{
				"Version": "2012-10-17",
				"Statement": [{
					"Effect": "Allow",
					"Action": ["s3:GetObject", "s3:ListBucket"],
					"Resource": ["%s", "%s/*"]
				}]
			}`, arn, arn), nil
		}).(pulumi.StringOutput),
	})
	if err != nil {
		return nil, err
	}

	profile, err := iam.NewInstanceProfile(ctx, name+"-ec2-profile", &iam.InstanceProfileArgs{
		Role: role.Name,
		Tags: pulumi.StringMap{"Name": pulumi.String(name + "-ec2-profile")},
	})
	if err != nil {
		return nil, err
	}

	return &IamResult{
		InstanceProfileName: profile.Name,
	}, nil
}
