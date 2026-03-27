package main

import (
	"fmt"

	"github.com/pulumi/pulumi-aws/sdk/v7/go/aws/s3"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type AwsS3Website struct {
	pulumi.ResourceState
	Url      pulumi.StringOutput // the S3 website url.
	BucketID pulumi.IDOutput     // the bucket ID.
}

type AwsS3WebsiteArgs struct {
	Files []string // a list of files to serve.
}

func NewAwsS3Website(ctx *pulumi.Context, name string, args AwsS3WebsiteArgs, opts ...pulumi.ResourceOption) (*AwsS3Website, error) {
	self := &AwsS3Website{}
	err := ctx.RegisterComponentResource("quickstart:index:AwsS3Website", name, self, opts...)
	if err != nil {
		return nil, err
	}

	// Create an AWS resource (S3 Bucket)
	bucket, err := s3.NewBucket(ctx, "my-bucket", nil,
		pulumi.Parent(self))
	if err != nil {
		return nil, err
	}

	// Turn the bucket into a website:
	website, err := s3.NewBucketWebsiteConfiguration(ctx, "website", &s3.BucketWebsiteConfigurationArgs{
		Bucket: bucket.ID(),
		IndexDocument: &s3.BucketWebsiteConfigurationIndexDocumentArgs{
			Suffix: pulumi.String("index.html"),
		},
	}, pulumi.Parent(self))
	if err != nil {
		return nil, err
	}

	// Permit access control configuration:
	ownershipControls, err := s3.NewBucketOwnershipControls(ctx, "ownership-controls", &s3.BucketOwnershipControlsArgs{
		Bucket: bucket.ID(),
		Rule: &s3.BucketOwnershipControlsRuleArgs{
			ObjectOwnership: pulumi.String("ObjectWriter"),
		},
	}, pulumi.Parent(self))
	if err != nil {
		return nil, err
	}

	// Enable public access to the website:
	publicAccessBlock, err := s3.NewBucketPublicAccessBlock(ctx, "public-access-block", &s3.BucketPublicAccessBlockArgs{
		Bucket:          bucket.ID(),
		BlockPublicAcls: pulumi.Bool(false),
	}, pulumi.Parent(self))
	if err != nil {
		return nil, err
	}

	// Create an S3 Bucket object for each file; note the changes to name/source:
	for _, file := range args.Files {
		_, err = s3.NewBucketObject(ctx, file, &s3.BucketObjectArgs{
			Bucket:      bucket.ID(),
			Source:      pulumi.NewFileAsset(file),
			ContentType: pulumi.String("text/html"),
			Acl:         pulumi.String("public-read"),
		}, pulumi.DependsOn([]pulumi.Resource{ownershipControls, publicAccessBlock}), pulumi.Parent(self))
		if err != nil {
			return nil, err
		}
	}

	// Export the bucket's autoassigned URL:
	self.Url = website.WebsiteEndpoint.ApplyT(func(websiteEndpoint string) (string, error) {
		return fmt.Sprintf("http://%v", websiteEndpoint), nil
	}).(pulumi.StringOutput)

	// Export the bucket ID:
	self.BucketID = bucket.ID()

	ctx.RegisterResourceOutputs(self, pulumi.Map{"url": self.Url, "bucketId": self.BucketID}) // Signal component completion.
	return self, nil
}
