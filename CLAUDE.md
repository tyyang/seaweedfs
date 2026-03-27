# SeaweedFS — CLAUDE.md

This file provides guidance for AI assistants working in this repository.

---

## Repository Overview

SeaweedFS is a distributed object storage system written in Go. It provides:
- A fast simple-volume-based object store (master + volume servers)
- An optional hierarchical filer layer with 25+ pluggable metadata backends
- An S3-compatible API gateway with IAM
- FUSE mount, WebDAV, SFTP, and message queue features
- Erasure coding, cloud tiering, and replication

The single Go module is `github.com/seaweedfs/seaweedfs` (Go 1.24.0+, toolchain 1.24.1).

---

## Directory Structure

```
seaweedfs/
├── weed/                  # All production Go source code
│   ├── command/           # CLI subcommand implementations (one file per command)
│   ├── server/            # HTTP + gRPC server logic (master, filer, volume)
│   ├── storage/           # Volume engine: needles, indexes, erasure coding
│   ├── filer/             # Filer layer: metadata, directory tree, backends
│   ├── s3api/             # S3-compatible API + multipart + ACL + bucket policy
│   ├── pb/                # Protobuf definitions (.proto) + generated Go code
│   ├── topology/          # Volume topology and placement decisions
│   ├── operation/         # Core operations (assign, upload, lookup)
│   ├── security/          # JWT auth, TLS guard
│   ├── iam/ iamapi/       # IAM policy engine + HTTP handlers
│   ├── mq/                # Distributed message queue (broker, agent, Kafka GW)
│   ├── notification/      # Event notifications (SQS, Pub/Sub, Kafka, webhooks)
│   ├── remote_storage/    # Cloud backend adapters (S3, GCS, Azure)
│   ├── replication/       # Replication sink/source logic
│   ├── kms/               # Key Management Service integrations
│   ├── mount/             # FUSE mount implementation
│   ├── sftpd/             # SFTP daemon
│   ├── wdclient/          # Master + filer gRPC client libraries
│   ├── cluster/           # Cluster node registry / discovery
│   ├── shell/             # Interactive shell commands
│   ├── admin/             # Web admin UI (Gin + Templ + Bootstrap)
│   ├── credential/        # Credential stores (filer_etc, memory, postgres)
│   ├── sequence/          # Distributed sequence generation
│   ├── stats/             # Prometheus metrics
│   ├── telemetry/         # Telemetry integration
│   ├── util/              # Shared utilities (17 sub-packages: logging, HTTP, crypto…)
│   ├── glog/              # Google-style levelled logging
│   ├── images/            # EXIF metadata, image resizing
│   ├── query/             # SQL-ish query engine (JSON + protobuf)
│   └── weed.go            # Binary entry point
├── test/                  # Integration test suites (docker-compose based)
│   ├── s3/                # S3 API conformance (19 test suites)
│   ├── kafka/             # Kafka pub/sub (12 suites)
│   ├── erasure_coding/    # EC integration tests
│   ├── fuse_integration/  # FUSE mount tests
│   └── …                  # kms, sftp, mq, postgres, etc.
├── docker/                # Dockerfiles + 25+ docker-compose variants
│   └── compose/           # Config templates (s3.json, filer.toml, replication.toml…)
├── k8s/charts/seaweedfs/  # Helm chart for Kubernetes deployment
├── .github/workflows/     # 40+ CI workflows (go.yml, s3, kafka, fuse, e2e…)
├── Makefile               # Top-level build automation
├── go.mod / go.sum        # Module dependencies
└── *.md                   # README, DESIGN, policy docs
```

---

## Build System

### Common Makefile Targets

| Target | Description |
|--------|-------------|
| `make install` | Build base `weed` binary (`cd weed; go install`) |
| `make full_install` | Build with all optional backends (elastic, gocdk, sqlite, ydb, tarantool, tikv, rclone) |
| `make test` | Run all unit tests with all optional build tags |
| `make server` | Start a local all-in-one dev server (master+filer+S3+volume) |
| `make benchmark` | Run S3 benchmark via minio/warp |
| `make admin-generate` | Regenerate Templ HTML templates for admin UI |
| `make admin-build` | Build admin component |
| `make admin-dev` | Run admin UI dev server with hot-reload |
| `make admin-test` | Test admin component |

### Build Tags (optional storage backends)

```
elastic   – Elasticsearch filer metadata backend
gocdk     – gocloud.dev pub/sub backends
sqlite    – SQLite filer metadata backend
ydb       – YDB metadata backend
tarantool – Tarantool backend
tikv      – TiKV backend
rclone    – Rclone remote storage
```

Use `-tags "elastic gocdk sqlite ydb tarantool tikv rclone"` for a full build.

### Admin UI (Templ Templates)

The admin component in `weed/admin/` uses [Templ](https://templ.guide/) for type-safe HTML templates. Generated `*_templ.go` files must be regenerated after changing `.templ` files:

```bash
go install github.com/a-h/templ/cmd/templ@latest
cd weed/admin && templ generate
# or from root:
make admin-generate
```

---

## Development Workflows

### Running a Local Dev Server

```bash
make server
# Starts: master(:9333) + volume + filer + S3(:8000) + metrics(:9324)
# Uses docker/compose/s3.json for S3 credentials
```

### Running Tests

```bash
# All unit tests (fast, no external deps):
cd weed && go test ./...

# With all build tags:
make test

# Single package:
cd weed && go test ./storage/...
cd weed && go test ./s3api/...

# Integration tests (require Docker):
cd test/s3 && docker-compose up --abort-on-container-exit
```

### Protobuf Regeneration

Proto files live in `weed/pb/`. Generated Go code is committed to the repo. To regenerate after changing `.proto` files:

```bash
cd weed/pb
protoc --go_out=. --go-grpc_out=. *.proto
```

The generated packages are imported as e.g. `filer_pb "github.com/seaweedfs/seaweedfs/weed/pb/filer_pb"`.

---

## Code Conventions

### Package Layout

- One primary concern per package; large packages are split across multiple files by responsibility (e.g. `filer_deletion.go`, `filer_server_handlers.go`).
- Test files are colocated: `foo_test.go` next to `foo.go`, same package or `_test` package.
- Build-tag gated files use the `//go:build <tag>` directive at the top.

### Naming

| Pattern | Example |
|---------|---------|
| File references | `Needle` (immutable stored object) |
| Storage container | `Volume` (VolumeId uint32) |
| Filer metadata | `Entry` (file or directory) |
| Chunk reference | `FileChunk` (needle location within volume) |
| EC shard | `EcVolumeShard` |
| gRPC generated types | `filer_pb.Entry`, `master_pb.VolumeInformation` |

- Package names: lowercase, single word (`s3api`, `filer`, `topology`).
- Proto-generated packages are suffixed `_pb` by convention (`filer_pb`, `master_pb`, `volume_server_pb`).
- Interfaces: `Store`, `Filer`, `Command` — no `I` prefix.

### Error Handling

- Return `(T, error)` explicitly; wrap with context using `fmt.Errorf("...: %w", err)`.
- Package-level sentinel errors: `var ErrVolumeNotFound = errors.New(...)`.
- Don't use `panic` except in truly unrecoverable init paths.

### gRPC / HTTP

- All inter-server RPCs use gRPC (generated from `weed/pb/*.proto`).
- HTTP is used for client-facing APIs (S3, WebDAV, admin, filer HTTP API).
- JWT security guard wraps handlers in `weed/security/`.

### Logging

- Use the vendored `glog` package: `glog.V(2).Infof(...)`, `glog.Errorf(...)`.
- Verbosity levels: 0 (errors/warnings), 1 (info), 2+ (debug).

---

## Key Proto Services

| Proto file | Service | Notable RPCs |
|------------|---------|--------------|
| `filer.proto` | SeaweedFiler | ListEntries, CreateEntry, SubscribeMetadata, DistributedLock |
| `master.proto` | Seaweed | Assign, LookupVolume, SendHeartbeat, VolumeGrow, RaftAddServer |
| `volume_server.proto` | VolumeServer | AllocateVolume, WriteNeedle, ReadNeedleBlob, VolumeEcShardRead |
| `mq_broker.proto` | SeaweedMessaging | PublishMessage, SubscribeMessage |
| `s3.proto` | SeaweedS3 | S3 gateway control plane |

---

## Component Port Defaults

| Component | Default Port |
|-----------|-------------|
| Master | 9333 (HTTP), 19333 (gRPC) |
| Volume Server | 8080 (HTTP), 18080 (gRPC) |
| Filer | 8888 (HTTP), 18888 (gRPC) |
| S3 Gateway | 8333 |
| Admin UI | 23646 |
| Metrics (Prometheus) | 9324 |
| WebDAV | 7333 |
| SFTP | 2222 |
| MQ Broker | 17777 |

---

## CI/CD Overview

GitHub Actions workflows in `.github/workflows/`:

| Workflow | Trigger | What it Tests |
|----------|---------|---------------|
| `go.yml` | push/PR | `go test ./...`, linting |
| `s3-go-tests.yml` | push/PR | S3 API conformance |
| `s3-iam-tests.yml` | push/PR | S3 IAM authorization |
| `kafka-tests.yml` | push/PR | Message queue Kafka gateway |
| `fuse-integration.yml` | push/PR | FUSE mount operations |
| `e2e.yml` | push/PR | End-to-end cluster scenarios |
| `ec-integration-tests.yml` | push/PR | Erasure coding |
| `postgres-tests.yml` | push/PR | PostgreSQL metadata backend |
| `codeql.yml` | schedule | Security analysis |
| `container_*.yml` | push/PR | Multi-platform Docker builds |
| `binaries_*.yml` | tag | Cross-platform release binaries |

---

## Adding a New Command

1. Create `weed/command/mycommand.go` implementing the `Command` interface (see `command.go`).
2. Register it in `weed/command/imports.go`.
3. Add tests in `weed/command/mycommand_test.go`.

## Adding a New Filer Metadata Backend

1. Implement `filer.AbstractSqlStore` or the `filer.Store` interface.
2. Place the implementation in `weed/filer/` (e.g., `filer_store_mydb.go`).
3. Wire it up in the filer factory / store selection logic.
4. Add a compose file in `docker/` for integration testing.

---

## Docker / Local Integration Testing

```bash
# All-in-one dev cluster
docker-compose -f docker/seaweedfs-dev-compose.yml up

# S3-focused cluster
docker-compose -f docker/compose/local-dev-compose.yml up

# With Kafka
docker-compose -f docker/compose/local-kafka-compose.yml up
```

Configuration template files live in `docker/compose/` (`.toml`, `.json`).

---

## Dependencies of Note

| Dependency | Purpose |
|------------|---------|
| `github.com/gorilla/mux` | HTTP routing |
| `github.com/seaweedfs/raft` | Raft consensus (master HA) |
| `github.com/Shopify/sarama` | Kafka client |
| `github.com/jackc/pgx/v5` | PostgreSQL driver |
| `github.com/linxGnu/grocksdb` | RocksDB bindings |
| `github.com/klauspost/reedsolomon` | Erasure coding |
| `github.com/aws/aws-sdk-go` | AWS S3, SQS, KMS |
| `cloud.google.com/go/storage` | GCS backend |
| `github.com/prometheus/client_golang` | Metrics |
| `github.com/a-h/templ` | Admin UI HTML templates |
| `github.com/gin-gonic/gin` | Admin UI HTTP framework |
| `github.com/spf13/viper` | Configuration loading |
| `github.com/bwmarrin/snowflake` | Distributed ID generation |
| `github.com/golang/protobuf` | Protobuf serialization |

---

## Important Files for Orientation

| File | Purpose |
|------|---------|
| `weed/weed.go` | Binary entry point; registers all commands |
| `weed/command/command.go` | Command registry and help system |
| `weed/command/server.go` | All-in-one `weed server` command |
| `weed/storage/store.go` | Core volume store logic |
| `weed/filer/filer.go` | Filer entry point |
| `weed/server/master_server.go` | Master server implementation |
| `weed/server/filer_server.go` | Filer HTTP/gRPC server |
| `weed/server/volume_server.go` | Volume server implementation |
| `weed/s3api/s3api_server.go` | S3 gateway server |
| `weed/pb/filer.proto` | Filer service protobuf definition |
| `weed/pb/master.proto` | Master service protobuf definition |
| `docker/compose/s3.json` | S3 credential config template |
| `docker/compose/filer.toml` | Filer backend config template |
