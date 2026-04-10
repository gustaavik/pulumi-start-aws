package component

import (
	"fmt"

	"github.com/pulumi/pulumi-aws/sdk/v7/go/aws/ec2"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type ApiServerArgs struct {
	SubnetID         pulumi.IDOutput
	SecurityGroupID  pulumi.IDOutput
	TailscaleAuthKey pulumi.StringInput
	DbEndpoint       pulumi.StringOutput
	KeyName          pulumi.StringOutput
}

type ApiServerResult struct {
	PrivateIP pulumi.StringOutput
}

func NewApiServer(ctx *pulumi.Context, name string, args ApiServerArgs) (*ApiServerResult, error) {
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

	userData := pulumi.All(args.TailscaleAuthKey, args.DbEndpoint).ApplyT(func(vals []interface{}) (string, error) {
		authKey := vals[0].(string)
		dbEndpoint := vals[1].(string)
		return fmt.Sprintf(`#!/bin/bash
set -e

# Install Node.js 20 LTS via NodeSource
curl -fsSL https://deb.nodesource.com/setup_20.x | bash -
apt-get install -y nodejs socat

# Create app directory
mkdir -p /opt/api
cd /opt/api

# Write package.json
cat > package.json << 'PKGJSON'
{
  "name": "api",
  "version": "1.0.0",
  "main": "index.js",
  "dependencies": {
    "express": "^4.21.0"
  }
}
PKGJSON

# Install dependencies
npm install --production

# Write the Express app
cat > index.js << 'APPJS'
const express = require('express');
const app = express();
const PORT = 3000;

app.use(express.json());

app.get('/health', (req, res) => {
  res.json({ status: 'ok', timestamp: new Date().toISOString() });
});

app.get('/api', (req, res) => {
  res.json({ message: 'Hello from private API', host: require('os').hostname() });
});

app.listen(PORT, '0.0.0.0', () => {
  console.log('API listening on port ' + PORT);
});
APPJS

# Create API systemd service
cat > /etc/systemd/system/api.service << 'SVCEOF'
[Unit]
Description=Node.js Express API
After=network.target

[Service]
Type=simple
User=nobody
WorkingDirectory=/opt/api
ExecStart=/usr/bin/node /opt/api/index.js
Restart=on-failure
RestartSec=5
StandardOutput=journal
StandardError=journal
Environment=NODE_ENV=production

[Install]
WantedBy=multi-user.target
SVCEOF

chown -R nobody:nogroup /opt/api
systemctl daemon-reload
systemctl enable api
systemctl start api

# Install Tailscale and join tailnet
curl -fsSL https://tailscale.com/install.sh | sh
tailscale up --authkey=%s

# TCP proxy: expose RDS on port 5432 via Tailscale
cat > /etc/systemd/system/db-proxy.service << DBEOF
[Unit]
Description=TCP proxy to RDS PostgreSQL
After=network.target

[Service]
Type=simple
ExecStart=/usr/bin/socat TCP-LISTEN:5432,fork,reuseaddr TCP:%s
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
DBEOF

systemctl daemon-reload
systemctl enable db-proxy
systemctl start db-proxy
`, authKey, dbEndpoint), nil
	}).(pulumi.StringOutput)

	instance, err := ec2.NewInstance(ctx, name+"-api-server", &ec2.InstanceArgs{
		Ami:                     pulumi.String(ami.Id),
		InstanceType:            pulumi.String("t3.micro"),
		SubnetId:                args.SubnetID,
		VpcSecurityGroupIds:     pulumi.StringArray{args.SecurityGroupID},
		KeyName:                 args.KeyName,
		UserData:                userData,
		UserDataReplaceOnChange: pulumi.Bool(true),
		Tags:                    pulumi.StringMap{"Name": pulumi.String(name + "-api-server")},
	})
	if err != nil {
		return nil, err
	}

	return &ApiServerResult{
		PrivateIP: instance.PrivateIp,
	}, nil
}
