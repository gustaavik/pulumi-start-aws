package main

import (
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {
		// Turn the bucket into a website:
		website, err := NewAwsS3Website(ctx, "my-website", AwsS3WebsiteArgs{
			Files: []string{"index.html"},
		})
		if err != nil {
			return err
		}

		// Export the name of the bucket
		ctx.Export("bucketName", website.BucketID)
		// Export the bucket's autoassigned URL:
		ctx.Export("url", website.Url)
		return nil
	})

}
