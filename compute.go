package main

import (
	"fmt"

	"github.com/pulumi/pulumi-aws/sdk/v7/go/aws/ec2"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type WebServerArgs struct {
	SubnetID            pulumi.IDOutput
	SecurityGroupID     pulumi.IDOutput
	InstanceProfileName pulumi.StringOutput
	BucketID            pulumi.IDOutput
	ApiPrivateIP        pulumi.StringOutput
	KeyName             pulumi.StringOutput
}

type WebServerResult struct {
	PublicIP  pulumi.StringOutput
	PrivateIP pulumi.StringOutput
}

func NewWebServer(ctx *pulumi.Context, name string, args WebServerArgs) (*WebServerResult, error) {
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

	userData := pulumi.All(args.BucketID, args.ApiPrivateIP).ApplyT(func(vals []interface{}) (string, error) {
		bucket := string(vals[0].(pulumi.ID))
		apiIP := vals[1].(string)
		return fmt.Sprintf(`#!/bin/bash
set -e
apt-get update -y
apt-get install -y nginx unzip curl

# Install AWS CLI v2 (not available as apt package on Ubuntu 24.04)
curl -fsSL "https://awscli.amazonaws.com/awscli-exe-linux-x86_64.zip" -o /tmp/awscliv2.zip
unzip -q /tmp/awscliv2.zip -d /tmp
/tmp/aws/install --update
rm -rf /tmp/awscliv2.zip /tmp/aws

systemctl enable nginx

# Retry S3 copy — IAM credentials may take time to propagate
set +e
for i in $(seq 1 12); do
    if /usr/local/bin/aws s3 cp s3://%s/index.html /var/www/html/index.html; then
        break
    fi
    sleep 5
done
set -e

if [ ! -f /var/www/html/index.html ]; then
    echo '<h1>S3 copy failed</h1><p>Could not fetch index.html from S3.</p>' > /var/www/html/index.html
fi

# Configure nginx to reverse-proxy /api to the private API server
cat > /etc/nginx/sites-available/default << 'NGINXEOF'
server {
    listen 80 default_server;

    root /var/www/html;
    index index.html;

    location / {
        try_files $uri $uri/ =404;
    }

    location /api {
        proxy_pass http://%s:3000;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    }
}
NGINXEOF

systemctl restart nginx
`, bucket, apiIP), nil
	}).(pulumi.StringOutput)

	instance, err := ec2.NewInstance(ctx, name+"-webserver", &ec2.InstanceArgs{
		Ami:                      pulumi.String(ami.Id),
		InstanceType:             pulumi.String("t3.micro"),
		SubnetId:                 args.SubnetID,
		VpcSecurityGroupIds:      pulumi.StringArray{args.SecurityGroupID},
		IamInstanceProfile:       args.InstanceProfileName,
		AssociatePublicIpAddress: pulumi.Bool(true),
		KeyName:                  args.KeyName,
		UserData:                 userData,
		UserDataReplaceOnChange:  pulumi.Bool(true),
		Tags:                     pulumi.StringMap{"Name": pulumi.String(name + "-webserver")},
	})
	if err != nil {
		return nil, err
	}

	return &WebServerResult{
		PublicIP:  instance.PublicIp,
		PrivateIP: instance.PrivateIp,
	}, nil
}
