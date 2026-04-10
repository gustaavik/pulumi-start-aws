package main

import (
	"fmt"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
)

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {
		cfg := config.New(ctx, "")
		tailscaleAuthKey := cfg.RequireSecret("tailscaleAuthKey")
		_ = cfg.RequireSecret("dbPassword")
		sshPublicKey := cfg.Require("sshPublicKey")

		dbUsername := cfg.Get("dbUsername")
		if dbUsername == "" {
			dbUsername = "postgres"
		}
		dbName := cfg.Get("dbName")
		if dbName == "" {
			dbName = "app"
		}

		// 1. SSH key pair for EC2 access
		kp, err := NewKeyPair(ctx, "main", sshPublicKey)
		if err != nil {
			return err
		}

		// 2. Networking: VPC, subnets, security groups
		net, err := NewNetwork(ctx, "main")
		if err != nil {
			return err
		}

		// 2. S3 storage for static website files
		storage, err := NewStorage(ctx, "main", StorageArgs{
			Files: []string{"index.html"},
		})
		if err != nil {
			return err
		}

		// 3. IAM role so EC2 can read from S3
		iamRes, err := NewIam(ctx, "main", IamArgs{
			BucketArn: storage.BucketArn,
		})
		if err != nil {
			return err
		}

		// 4. NAT instance for private subnet outbound internet
		err = NewNat(ctx, "main", NatArgs{
			VpcID:          net.VpcID,
			PublicSubnetID: net.PublicSubnetID,
			NatSgID:        net.NatSgID,
			PrivateSubnetIDs: []pulumi.IDOutput{
				net.PrivateSubnetID,
				net.DbSubnetIDs[0],
				net.DbSubnetIDs[1],
			},
		})
		if err != nil {
			return err
		}

		// 5. RDS PostgreSQL in private DB subnets
		// db, err := NewDatabase(ctx, "main", DatabaseArgs{
		// 	DbSubnetIDs:     net.DbSubnetIDs,
		// 	SecurityGroupID: net.DatabaseSgID,
		// 	DbName:          dbName,
		// 	DbUsername:      dbUsername,
		// 	DbPassword:      dbPassword,
		// })
		// if err != nil {
		// 	return err
		// }

		// 6. Node.js API server in private app subnet (Tailscale exposes API + DB proxy)
		apiServer, err := NewApiServer(ctx, "main", ApiServerArgs{
			SubnetID:         net.PrivateSubnetID,
			SecurityGroupID:  net.ApiSgID,
			TailscaleAuthKey: tailscaleAuthKey,
			DbEndpoint:       pulumi.Sprintf("%s:5432", "mydb.cluster-xyz.us-west-2.rds.amazonaws.com"),
			KeyName:          kp.KeyName,
		})
		if err != nil {
			return err
		}

		// 7. Web server in public subnet (proxies /api to the private API server)
		webServer, err := NewWebServer(ctx, "main", WebServerArgs{
			SubnetID:            net.PublicSubnetID,
			SecurityGroupID:     net.WebserverSgID,
			InstanceProfileName: iamRes.InstanceProfileName,
			BucketID:            storage.BucketID,
			ApiPrivateIP:        apiServer.PrivateIP,
			KeyName:             kp.KeyName,
		})
		if err != nil {
			return err
		}

		// 8. Tailscale subnet router - advertises VPC to tailnet
		_, err = NewTailscaleRouter(ctx, "main", TailscaleRouterArgs{
			SubnetID:        net.PublicSubnetID,
			SecurityGroupID: net.RouterSgID,
			AuthKey:         tailscaleAuthKey,
			VpcCidr:         vpcCidr,
			KeyName:         kp.KeyName,
		})
		if err != nil {
			return err
		}

		ctx.Export("bucketName", storage.BucketID)
		ctx.Export("websiteUrl", webServer.PublicIP.ApplyT(func(ip string) string {
			return fmt.Sprintf("http://%s", ip)
		}).(pulumi.StringOutput))
		// ctx.Export("dbEndpoint", db.Endpoint)
		ctx.Export("sshAccess", webServer.PrivateIP.ApplyT(func(ip string) string {
			return fmt.Sprintf("ssh ubuntu@%s (via Tailscale)", ip)
		}).(pulumi.StringOutput))
		ctx.Export("apiUrl", apiServer.PrivateIP.ApplyT(func(ip string) string {
			return fmt.Sprintf("http://%s:3000 (VPC/Tailscale only)", ip)
		}).(pulumi.StringOutput))

		return nil
	})
}
