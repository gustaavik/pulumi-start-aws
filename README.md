# pulumi-start-aws

Multi-tier AWS infrastructure provisioned with Pulumi in Go. The stack deploys a public-facing Nginx web server that reverse-proxies to a private Node.js API, with optional RDS PostgreSQL, a NAT instance for private subnet outbound access, and Tailscale VPN for secure remote connectivity — no bastion host or exposed SSH ports required.

## Architecture

```
                         ┌──────────────────┐
                         │     Internet     │
                         └────────┬─────────┘
                                  │ HTTP :80
                         ┌────────▼─────────┐       ┌──────────────────┐
                         │ Internet Gateway │       │ Tailscale Network│
                         └────────┬─────────┘       └────────┬─────────┘
                                  │                          │ UDP :41641
 ┌────────────────────────────────┼──────────────────────────┼──────────────────────┐
 │ AWS eu-west-1                  │                          │                      │
 │ ┌──────────────────────────────┼──────────────────────────┼────────────────────┐ │
 │ │ VPC 10.0.0.0/16              │                          │                    │ │
 │ │                              │                          │                    │ │
 │ │  ┌─────────────────────────────────────────────────────────────────────────┐ │ │
 │ │  │ Public Subnet — 10.0.10.0/24                                            │ │ │
 │ │  │                                                                         │ │ │
 │ │  │  ┌─────────────────┐  ┌──────────────────┐  ┌──────────────────────┐    │ │ │
 │ │  │  │   Web Server    │  │  NAT Instance    │  │  Tailscale Router    │    │ │ │
 │ │  │  │ t3.micro        │  │ t4g.nano         │  │ t3.nano              │    │ │ │
 │ │  │  │ Nginx :80       │  │ fck-nat ARM64    │  │ advertises VPC CIDR  │◄───┼─┼─┼
 │ │  │  └──┬──────────┬───┘  └──────▲───────────┘  └──────────────────────┘    │ │ │
 │ │  │     │          │             │                                          │ │ │
 │ │  └─────┼──────────┼─────────────┼──────────────────────────────────────────┘ │ │
 │ │        │          │             │                                            │ │
 │ │        │ proxy    │ s3:GetObject│ outbound                                   │ │
 │ │        │ /api     │ (IAM role)  │ via RT        ┌──────────────────────┐     │ │
 │ │        │ :3000    │             │               │    S3 Bucket         │     │ │
 │ │        │          └─────────────┼──────────────►│    static files      │     │ │
 │ │        │                        │               └──────────────────────┘     │ │
 │ │  ┌─────┼────────────────────────┼─────────────────────────────────────────┐  │ │
 │ │  │ Private App Subnet — 10.0.2.0/24                                       │  │ │
 │ │  │     │                        │                                         │  │ │
 │ │  │  ┌──▼──────────────────────┐ │                                         │  │ │
 │ │  │  │   API Server            ├─┘                                         │  │ │
 │ │  │  │ t3.micro · Express :3000│◄─────── Tailscale node                    │  │ │
 │ │  │  │ socat :5432             │                                           │  │ │
 │ │  │  └──────────┬──────────────┘                                           │  │ │
 │ │  │             │                                                          │  │ │
 │ │  └─────────────┼──────────────────────────────────────────────────────────┘  │ │
 │ │                │ socat :5432                                                 │ │
 │ │  ┌─────────────┼──────────────────────────────────────────────────────────┐  │ │
 │ │  │ Private DB Subnets — multi-AZ                                          │  │ │
 │ │  │             │                                                          │  │ │
 │ │  │  ┌──────────▼──────────────┐                                           │  │ │
 │ │  │  │   RDS PostgreSQL        │     ┌──────────────┐  ┌──────────────┐    │  │ │
 │ │  │  │ db.t4g.micro · PG 16    │     │ DB Subnet A  │  │ DB Subnet B  │    │  │ │
 │ │  │  │ (currently disabled)    │     │ 10.0.3.0/24  │  │ 10.0.4.0/24  │    │  │ │
 │ │  │  └─────────────────────────┘     │ AZ-a         │  │ AZ-b         │    │  │ │
 │ │  │                                  └──────────────┘  └──────────────┘    │  │ │
 │ │  └────────────────────────────────────────────────────────────────────────┘  │ │
 │ │                                                                              │ │
 │ └──────────────────────────────────────────────────────────────────────────────┘ │
 │                                                                                  │
 └──────────────────────────────────────────────────────────────────────────────────┘
```

## Infrastructure Components

| Component        | Type | Size                            | Subnet                | Purpose                                                                        |
| ---------------- | ---- | ------------------------------- | --------------------- | ------------------------------------------------------------------------------ |
| Web Server       | EC2  | t3.micro (Ubuntu 24.04)         | Public                | Nginx reverse proxy — serves static HTML from S3, proxies `/api` to API server |
| API Server       | EC2  | t3.micro (Ubuntu 24.04)         | Private App           | Node.js Express on :3000, Tailscale node, socat proxy to RDS :5432             |
| NAT Instance     | EC2  | t4g.nano (fck-nat AL2023 ARM64) | Public                | Outbound internet for all private subnets (source/dest check disabled)         |
| Tailscale Router | EC2  | t3.nano (Ubuntu 24.04)          | Public                | Advertises VPC CIDR to tailnet for remote access                               |
| RDS PostgreSQL   | RDS  | db.t4g.micro (PG 16)            | Private DB (multi-AZ) | Relational database — **currently disabled** in main.go                        |
| S3 Bucket        | S3   | —                               | —                     | Stores static website files (index.html)                                       |

## Network & Security

### Security Groups

| Security Group      | Inbound                                    | Outbound |
| ------------------- | ------------------------------------------ | -------- |
| webserver-sg        | TCP :80 from `0.0.0.0/0`, TCP :22 from VPC | All      |
| api-sg              | TCP :3000 from VPC, TCP :22 from VPC       | All      |
| nat-sg              | All traffic from VPC                       | All      |
| tailscale-router-sg | UDP :41641 from `0.0.0.0/0`                | All      |
| database-sg         | TCP :5432 from VPC                         | All      |

### Data Flow

- **Public HTTP** — Internet &rarr; IGW &rarr; Web Server (:80) &rarr; Nginx serves `/` from S3 static files or proxies `/api` &rarr; API Server (:3000)
- **Private outbound** — Private subnets &rarr; private route table &rarr; NAT Instance &rarr; IGW &rarr; Internet
- **Tailscale access** — Tailscale client &rarr; Tailscale Router (UDP :41641) &rarr; subnet route advertises `10.0.0.0/16` &rarr; any VPC resource
- **Database** — API Server &rarr; socat :5432 &rarr; RDS PostgreSQL endpoint

## Prerequisites

- Go 1.21+
- [Pulumi CLI](https://www.pulumi.com/docs/install/) v3+
- AWS credentials configured (`aws configure` or environment variables)
- A [Tailscale](https://tailscale.com/) account with an auth key
- An ED25519 SSH key pair (`ssh-keygen -t ed25519`)

## Configuration

| Key                                 | Type   | Required | Default     | Description                                                 |
| ----------------------------------- | ------ | -------- | ----------- | ----------------------------------------------------------- |
| `aws:region`                        | string | yes      | `eu-west-1` | AWS region                                                  |
| `pulumi-start-aws:tailscaleAuthKey` | secret | yes      | —           | Tailscale auth key for router + API server                  |
| `pulumi-start-aws:sshPublicKey`     | string | yes      | —           | ED25519 public key for EC2 SSH access                       |
| `pulumi-start-aws:dbPassword`       | secret | yes      | —           | RDS PostgreSQL password (required even when DB is disabled) |
| `pulumi-start-aws:dbUsername`       | string | no       | `postgres`  | PostgreSQL username                                         |
| `pulumi-start-aws:dbName`           | string | no       | `app`       | PostgreSQL database name                                    |

## Getting Started

```bash
# Clone and enter the project
git clone https://github.com/gustaavik/pulumi-start-aws && cd pulumi-start-aws

# Create a stack
pulumi stack init dev

# Set required config
pulumi config set aws:region eu-west-1
pulumi config set --secret pulumi-start-aws:tailscaleAuthKey tskey-auth-...
pulumi config set pulumi-start-aws:sshPublicKey "ssh-ed25519 AAAA..."
pulumi config set --secret pulumi-start-aws:dbPassword "your-db-password"

# Preview and deploy
pulumi preview
pulumi up
```

## Stack Outputs

| Output       | Description                                                      |
| ------------ | ---------------------------------------------------------------- |
| `websiteUrl` | `http://<public-ip>` — public HTTP endpoint                      |
| `apiUrl`     | `http://<private-ip>:3000` — reachable via VPC or Tailscale only |
| `sshAccess`  | SSH command via Tailscale                                        |
| `bucketName` | S3 bucket name                                                   |

## Project Layout

```
.
├── Pulumi.yaml              Project metadata
├── Pulumi.dev.yaml          Dev stack config (secrets encrypted)
├── main.go                  Entry point — wires all components
├── index.html               Static HTML uploaded to S3
├── go.mod / go.sum          Go module dependencies
└── component/
    ├── network.go           VPC, subnets, IGW, route tables, security groups
    ├── compute.go           Web server EC2 (Nginx + S3 + reverse proxy)
    ├── api.go               API server EC2 (Express + Tailscale + socat)
    ├── nat.go               NAT instance (fck-nat ARM64) + private routing
    ├── tailscale.go         Tailscale subnet router EC2
    ├── storage.go           S3 bucket + object upload
    ├── iam.go               EC2 IAM role + S3 read policy
    ├── database.go          RDS PostgreSQL (disabled in main.go)
    └── keypair.go           ED25519 SSH key pair
```

## Enabling RDS

The database module is fully implemented in `database.go` but the call is commented out in `main.go`. To enable it, uncomment the `NewDatabase` block (lines 70-79) and the `dbEndpoint` export (line 122) in `main.go`.

## Remote Access via Tailscale

Two access patterns are available:

1. **Subnet router** — The Tailscale Router instance advertises the entire VPC CIDR (`10.0.0.0/16`) to your tailnet. After accepting the route (`tailscale up --accept-routes`), you can reach any private IP directly: `ssh ubuntu@<private-ip>`.

2. **Direct node** — The API Server also joins the tailnet as a regular node, so it has its own Tailscale IP reachable without the subnet router.

## Links

- [Pulumi docs](https://www.pulumi.com/docs/)
- [Pulumi AWS provider](https://www.pulumi.com/registry/packages/aws/api-docs/)
- [fck-nat](https://github.com/AndrewGuenther/fck-nat) — NAT instance AMI
- [Tailscale subnet routers](https://tailscale.com/kb/1019/subnets)
