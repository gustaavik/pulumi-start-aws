package main

import (
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
)

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {
		cfg := config.New(ctx, "")
		zerotierNetworkID := cfg.Require("zerotierNetworkID")

		website, err := NewAwsS3Website(ctx, "my-website", AwsS3WebsiteArgs{
			Files:             []string{"index.html"},
			ZeroTierNetworkID: zerotierNetworkID,
		})
		if err != nil {
			return err
		}

		ctx.Export("bucketName", website.BucketID)
		ctx.Export("sshAccess", website.Url)
		return nil
	})
}
