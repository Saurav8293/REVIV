# REVIV: Automated Disaster Recovery Engine

> **Cold-standby disaster recovery engine in Go that provisions cloud infrastructure only upon failure — avoiding the exorbitant cost of 24/7 idle standby resources.**

[![Go Version](https://img.shields.io/badge/Go-1.24%2B-00ADD8?style=flat&logo=go)](https://golang.org)
[![PostgreSQL](https://img.shields.io/badge/PostgreSQL-16-336791?style=flat&logo=postgresql)](https://www.postgresql.org)
[![AWS SDK v2](https://img.shields.io/badge/AWS%20SDK-Go%20v2-232F3E?style=flat&logo=amazon-aws)](https://aws.amazon.com/sdk-for-go/)
[![WAL-G](https://img.shields.io/badge/WAL--G-v3.0.8-FF6F00?style=flat)](https://github.com/wal-g/wal-g)
[![React](https://img.shields.io/badge/React-19-61DAFB?style=flat&logo=react)](https://react.dev)

---

## 1. Executive Summary & Problem Solved

Traditional cloud disaster recovery typically relies on **Active-Active** or **Active-Passive (Hot Standby)** setups — requiring continuous, duplicate compute instances and managed multi-AZ database replicas (e.g. AWS RDS Multi-AZ) running 24/7. For early-stage companies and cost-sensitive workloads, this adds **$500 to $2,000+/month** in pure idle overhead.

**REVIV reverses this paradigm with Cold-Standby Disaster Recovery**:
1. Run everyday production workloads on low-cost compute (e.g. bare-metal VPS at $15–$40/month).
2. Continuously stream PostgreSQL Write-Ahead Logs (WAL) and daily base backups to the customer's own AWS S3 bucket via **WAL-G** (near-zero storage cost).
3. Concurrently monitor primary health with **`drwatch`** using goroutines, tickers, mutexes, and threshold escalators.
4. Require **Operator Confirmation** to guarantee zero accidental failovers or split-brain data corruption during temporary network blips.
5. Upon confirmed disaster, **automatically orchestrate AWS recovery via the AWS SDK for Go v2**: provision standby compute, restore the database to the exact crash point using WAL-G PITR, and switch **Route 53 DNS**.

| Metric | Traditional Hot Standby | REVIV Cold Standby Engine |
|---|---|---|
| **Idle Cloud Compute Cost** | $800 – $2,000 / month | **$0.00 / month** |
| **S3 Storage Cost** | Standard backup costs | Negligible (~$1–$5/mo) |
| **Recovery Point Objective (RPO)** | Near real-time | **< 30 seconds** (continuous WAL push) |
| **Recovery Time Objective (RTO)** | Instant (0-30s) | **~5 to 8 minutes** (on-demand compute spinup + WAL replay) |
| **False Failover Protection** | Complex split-brain risks | **Guaranteed** via threshold + operator token confirmation |

---

## 2. Architecture & System Components

```
┌────────────────────────────────────────────────────────────────────────┐
│ PRIMARY WORKLOAD (Bare Metal / VPS)                                    │
│   ├── PostgreSQL 16  (archive_mode = on, wal_level = replica)          │
│   │    └── wal-g wal-push "%p" ──────────────────┐                      │
│   └── drdaemon (Go agent)                        │ WAL Stream           │
│        └── HTTP POST Heartbeat (every 5s) ──┐    ▼                      │
└─────────────────────────────────────────────│── [AWS S3 Bucket]        │
                                              │   s3://<bucket>/wal/     │
                                              ▼                          │
┌─────────────────────────────────────────────────────────┐              │
│ REVIV CONTROL PLANE & WATCHER (drwatch)                 │              │
│   ├── Concurrent Heartbeat Watchdog (goroutines/mutex)  │              │
│   ├── Escalation Engine: HEALTHY ──► SUSPECT ──► DOWN   │              │
│   ├── Operator Confirmation Latch (Prevents Split-Brain)│              │
│   └── REST & WebSocket Telemetry API                    │              │
└───────────────────────────┬─────────────────────────────┘              │
                            │ On Disaster + Operator Token               │
                            ▼                                            │
┌─────────────────────────────────────────────────────────┐              │
│ AWS AUTOMATED FAILOVER ORCHESTRATOR (AWS SDK for Go v2) │              │
│   1. ec2:RunInstances (Launch Standby Compute)          │              │
│   2. S3 WAL-G Point-in-Time Restore ◄───────────────────┼──────────────┘
│      (wal-g backup-fetch + continuous wal-fetch replay) │
│   3. route53:ChangeResourceRecordSets (DNS Cutover)     │
└─────────────────────────────────────────────────────────┘
```

### Component Breakdown

1. **`cmd/drdaemon` (Primary Node Agent)**:
   - Lightweight Go agent running on the primary server.
   - Concurrently monitors PostgreSQL connection status, disk availability, and last WAL archive timestamp.
   - Pings `drwatch` with a structured telemetry payload every 5 seconds.
   - Exposes local diagnostic endpoint on `:8090/status`.

2. **`cmd/drwatch` (Heartbeat Monitor & Watchdog Engine)**:
   - Concurrent heartbeat monitor built with Go goroutines, tickers, and `sync.RWMutex`.
   - **Threshold Escalation**:
     - `0 - 2 misses`: Node marked `HEALTHY`.
     - `3 - 4 misses`: Escalates to `SUSPECT`, sends early alert webhook.
     - `5+ misses (>= 30s)`: Declares node `DOWN`, locks trigger with a unique `OperatorConfirmationToken`.
   - Provides REST endpoints for live status, metrics, drills, and the React Control Plane.

3. **`cmd/drctl` (Operator Disaster Recovery CLI)**:
   - `drctl status`: Renders terminal status table of primary health, backup lag, and standby readiness.
   - `drctl failover`: Prompts operator for confirmation, executes AWS SDK recovery, and prints execution report.
   - `drctl dry-run`: Non-destructive drill simulating failover steps without touching live cloud resources.
   - `drctl backup-check`: Inspects S3 bucket, validates base backup and continuous WAL segment continuity.

4. **`internal/awsops` (AWS SDK v2 Automation)**:
   - **EC2 Compute Provisioning**: Runs `RunInstances` with on-demand user-data script to initialize Docker and PostgreSQL.
   - **Route 53 DNS Switch**: Dispatches `ChangeResourceRecordSets` with `UPSERT` and 60-second TTL to point domain to the standby instance IP.
   - **S3 & WAL-G Validation**: Verifies backup artifacts prior to cutover.

5. **`frontend/` (Modern Dark-Mode Operations Dashboard)**:
   - Built with **React 19, Vite, Lucide Icons, and Recharts**.
   - Displays real-time architecture topology, heartbeat radar pulses, telemetry graphs, and interactive chaos simulation controls.
   - Includes full Operator Confirmation modal and 6-step failover execution stepper with live RTO timer.

---

## 3. Directory Layout

```
Reviv/
├── bin/                          # Compiled binaries (drwatch, drdaemon, drctl)
├── cmd/
│   ├── drdaemon/main.go          # Primary node telemetry agent
│   ├── drwatch/main.go           # Concurrent watcher & control plane API
│   └── drctl/main.go             # Operator disaster recovery CLI
├── configs/
│   ├── dr.yaml                   # Active local/testing configuration
│   └── dr.example.yaml           # Production reference configuration
├── internal/
│   ├── awsops/                   # AWS SDK v2 EC2, Route53, and S3 automation
│   │   ├── ec2.go
│   │   ├── route53.go
│   │   ├── orchestrator.go
│   │   └── types.go
│   ├── config/                   # YAML parser & environment overrides
│   ├── heartbeat/                # Concurrent monitor, tickers, and mutex state
│   │   ├── client.go
│   │   ├── monitor.go
│   │   └── types.go
│   └── walg/                     # WAL-G S3 continuous archiving & PITR script generator
├── postgres-backup/              # PostgreSQL 16 container with WAL-G v3.0.8
│   ├── Dockerfile
│   ├── docker-compose.yml
│   ├── init.sql                  # Seed schema & test data
│   └── restore.sh                # Point-in-time recovery script
├── frontend/                     # React + Vite Disaster Recovery Control Plane
│   ├── src/
│   │   ├── App.jsx               # Control plane dashboard
│   │   └── index.css             # Glassmorphic dark design system
│   └── package.json
└── docs/                         # Architecture & interview documentation
```

---

## 4. Quickstart Guide

### 1. Build Binaries
```powershell
# Build all three Go binaries into bin/
go build -o bin/drwatch.exe ./cmd/drwatch
go build -o bin/drdaemon.exe ./cmd/drdaemon
go build -o bin/drctl.exe ./cmd/drctl
```

### 2. Start the DR Watcher
```powershell
.\bin\drwatch.exe --config configs/dr.yaml
```

### 3. Start the Primary Daemon (in another terminal)
```powershell
.\bin\drdaemon.exe --config configs/dr.yaml
```

### 4. Check Health via CLI
```powershell
.\bin\drctl.exe status
.\bin\drctl.exe backup-check
.\bin\drctl.exe dry-run
```

### 5. Launch the Web Dashboard
```powershell
cd frontend
npm run dev
```
Navigate to `http://localhost:5173` to view the real-time Control Plane.

---

## 5. Key Design Decisions & Architecture Tradeoffs

### 1. Why Cold-Standby instead of Hot-Standby?
Hot standby requires running compute and database replicas 24/7 in AWS, costing upwards of $1,000/month. For workloads where a 5 to 8-minute RTO is acceptable, REVIV avoids all idle compute costs by keeping compute at 0 instances during normal operation. Write-Ahead Logs are streamed continuously to S3 (which costs pennies), and compute is provisioned via the AWS SDK only when a true failure occurs.

### 2. Split-Brain Mitigation & False Failover Prevention
Network blips happen frequently on VPS and public cloud networks. If failover triggered on a single missed ping, it would cause split-brain data divergence where two separate database instances simultaneously accept writes. REVIV enforces a two-layer defense:
1. **Threshold Escalation:** A missed ping marks the node `SUSPECT`, not `DOWN`. It requires consecutive misses exceeding the threshold (e.g. 5 misses / 30+ seconds) before declaring an outage.
2. **Operator Confirmation Latch:** When marked `DOWN`, the watchdog engine locks and generates a one-time authorization token. Automated failover will not touch AWS resources without explicit operator confirmation (`drctl failover` prompt or API token) or configured multi-region consensus.

### 3. Point-in-Time Recovery (PITR) Mechanics
PostgreSQL writes every change to Write-Ahead Logs before committing data blocks to disk. WAL-G continuously pushes these compressed 16MB WAL segments to S3 (`archive_command = 'wal-g wal-push "%p"'`). On failover, the orchestrator pulls the latest full base backup (`wal-g backup-fetch`), injects `recovery.signal` with `restore_command = 'wal-g wal-fetch "%f" "%p"'`, and PostgreSQL replays transactions up to the crash timestamp, achieving an RPO under 30 seconds.

---

## License
MIT License. Open-source disaster recovery engine.
