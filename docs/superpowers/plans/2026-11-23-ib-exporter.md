# ib_exporter Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Go Prometheus + OTLP exporter exposing an InfiniBand fabric (switch/HCA ports, FEC, topology, SM) plus local HCA sysfs, labelled by `fabric`.

**Architecture:** Collection loop per fabric reads through swappable readers (diags CLI runner, sysfs, replay) into an immutable `Snapshot`, swapped into a `SnapshotStore`. An unchecked Prometheus collector and an OTLP observable bridge both read the store. Family-standard server, probes, config, Makefile and CI.

**Tech Stack:** Go 1.26.4, cobra, prometheus/client_golang, OpenTelemetry SDK, fsnotify, testify, golangci-lint, GoReleaser, MkDocs, Docker (`debian:trixie-slim`).

**Spec:** `docs/superpowers/specs/2026-11-23-ib-exporter-design.md`. Family references: `~/.claude/skills/exporter-standards/references/{stack,architecture,cicd,new-exporter-checklist}.md`. Mirror `pflex_exporter`/`ppdd_exporter` for layout where the spec is silent.

## Global Constraints

- Module `github.com/fjacquet/ib_exporter`, `go 1.26.4`, metric prefix `ib_`, port `9449`, binary/build_info `ib_exporter_build_info{version,goversion}`.
- `fabric` label on every series; one label-key set per metric name (union invariant, empty string when inapplicable). HCA series for unbound ports use `fabric=""`.
- Counters are cumulative `_total`; dashboards use `rate()`. Never reset counters (no `perfquery -r/-R`). Unreadable value = absent sample, never zero.
- Series identity = fabric + node GUID + port; LID only resolved per cycle from the latest topology.
- Data counters from PMA are units of 4 octets: multiply by 4 for `_bytes_total`.
- Read both basic `perfquery -a -l` and `perfquery -x -a -l`, merge (basic = errors, `-x` = 64-bit data/packets).
- `/livez` and `/readyz` static 200; `/health` always 200 with per-fabric body. Reserved URIs `/ /health /livez /readyz` rejected in `Validate()` via exported `ReservedURIs`, routes registered from it.
- Serve HTTP before the first collection cycle. `--once --debug` dumps all samples; `--trace` logs command and output (no secrets exist, but never log env).
- Sysfs/exec readers `//go:build linux` with non-linux stubs so darwin builds compile.
- No cluster available: every reader is developed against fixtures in `testdata/`. Items marked `[unverified]` in the spec stay open until real hardware.
- Image `debian:trixie-slim`, uid `10001` named user, `curl` HEALTHCHECK on `127.0.0.1/livez`. Dependabot `gomod` + `docker` only. CI via `fjacquet/ci@v1` callers.
- Semgrep clean, no inline suppressions. TDD. Tests assert via Prometheus gather and OTLP `ManualReader`.

## Review Focus

- Command output with `N/A`, empty or truncated lines, or a switch that does not answer the PMA: sample absent, `ib_collect_errors_total` increments, other nodes unaffected.
- Saturated 16/32-bit counters (`0xffff`, `0xffffffff`): still emitted (flatline is real data), but covered by a test so no one "fixes" it to zero.
- LID changes between topology snapshots: the next cycle resolves the new LID; a stale LID failure yields absent samples, not an error storm.
- Node names with spaces/quotes in `ibnetdiscover -p` and a node-name-map missing or malformed: parse succeeds, name falls back to GUID.
- A fabric whose HCA/umad is not readable (permission denied) or empty `fabrics:` config: `ib_up{fabric}=0` plus a clear error; with no fabrics and `localHCA: true` the exporter still serves HCA metrics.

## File Structure

```
main.go                          cobra CLI, wiring, signals
internal/config/                 config.go, validate.go, reload.go
internal/ib/                     model.go, snapshot.go, store.go, collector.go (loop),
                                 prometheus.go, otlp.go, metrics.go, buildinfo.go
internal/ib/diags/               runner.go, topology.go, perfquery.go, portinfo.go, sm.go, ibswinfo.go
internal/ib/sysfs/               hca_linux.go, hca_other.go
internal/ib/replay/              replay.go (fixtures behind same interfaces)
internal/server/                 server.go (mux, probes, health)
testdata/                        ibnetdiscover, perfquery, smpquery, sminfo, sysfs trees
deploy/ docker-compose*.yml prometheus.yml grafana/ docs/ Makefile Dockerfile* .goreleaser.yaml .github/
```

---

### Task 1: Module, config, Makefile contract

**Files:** Create `go.mod`, `internal/config/{config.go,validate.go}`, `Makefile`, `.gitignore`, `config.yaml`, `.env.example`; Test `internal/config/config_test.go`.

**Interfaces:**
- Produces: `type Config struct{Server{Port int; URI string}; Collection{Interval, TopologyInterval, PortInfoInterval time.Duration; MaxConcurrent, OptionalEvery int}; Fabrics []Fabric; Sources{LocalHCA bool; Ibswinfo{Enabled bool; Path string}}; Replay{Dir string}}`, `type Fabric struct{Name, CA string; Port int; NodeNameMap string}`, `func Load(path string) (*Config, error)`, `func (c *Config) Validate() error`, `var ReservedURIs map[string]bool`.

- [ ] **Step 1: Failing tests** `TestLoad_ExpandsEnv`, `TestValidate_RejectsReservedURI` (table over `/`, `/health`, `/livez`, `/readyz`, missing leading slash), `TestValidate_DuplicateFabricName`, `TestValidate_EmptyFabricsAllowedWithLocalHCA`, defaults (`port 9449`, `interval 60s`, `topologyInterval 10m`, `portInfoInterval 5m`, `maxConcurrent 4`, `optionalEvery 5`).
- [ ] **Step 2:** `go test ./internal/config/...` → FAIL (undefined).
- [ ] **Step 3:** Implement per signatures; YAML via `gopkg.in/yaml.v3`, `os.ExpandEnv` on string fields.
- [ ] **Step 4:** Copy the full Makefile target contract from `pflex_exporter` (`tools fmt-check fmt vet lint test test-race test-coverage vuln ci sure cli sbom release release-snapshot docker run-cli clean`) and the canonical `.gitignore`; `make ci` runs.
- [ ] **Step 5:** `go test -race ./internal/config/...` PASS; commit `feat(config): config loading and validation`.

### Task 2: Model, Snapshot, SnapshotStore

**Files:** Create `internal/ib/{model.go,snapshot.go,store.go}`; Test `internal/ib/store_test.go`.

**Interfaces:**
- Produces: `type Sample struct{Name string; Labels []Label; Value float64; Counter bool}`, `type Label struct{Key, Value string}`, `type Node struct{GUID, Name string; Type NodeType; LID uint16; Ports int}`, `type Link struct{GUID string; Port int; RemoteGUID string; RemotePort int; RemoteName string}`, `type FabricSnapshot struct{Fabric string; Up bool; Nodes []Node; Links []Link; Samples []Sample; At time.Time}`, `type Snapshot struct{Fabrics map[string]*FabricSnapshot; HCA []Sample; BuiltAt time.Time}`, `type SnapshotStore struct{}` with `Swap(*Snapshot)`, `Load() *Snapshot`.

- [ ] **Step 1:** Failing tests `TestStore_SwapIsAtomic` (concurrent readers under `-race`), `TestStore_LoadBeforeSwapReturnsEmpty`.
- [ ] **Step 2–4:** Run FAIL; implement RWMutex pointer swap, immutable snapshots; run PASS.
- [ ] **Step 5:** Commit `feat(ib): snapshot model and store`.

### Task 3: Diags runner and topology parser

**Files:** Create `internal/ib/diags/{runner.go,topology.go}`, `testdata/ibnetdiscover/*.txt`; Test `topology_test.go`.

**Interfaces:**
- Produces: `type Runner interface{ Run(ctx context.Context, f config.Fabric, tool string, args ...string) ([]byte, error) }`; `ExecRunner` (appends `-C f.CA -P f.Port`, honours `--trace`); `func ParseTopology(out []byte, nameMap map[string]string) ([]ib.Node, []ib.Link, error)`; `func LoadNodeNameMap(path string) (map[string]string, error)`.

- [ ] **Step 1:** Fixtures from rdma-core `ibnetdiscover -p` format (2 switches, 3 HCAs, node name with spaces/quotes, duplicate-name node). Failing tests `TestParseTopology_NodesAndLinks`, `TestParseTopology_NameWithSpacesAndQuotes`, `TestParseTopology_UnknownLineIgnored`, `TestLoadNodeNameMap_MissingFileReturnsEmpty`.
- [ ] **Step 2–4:** FAIL → implement (text format isolated here only) → PASS. `ExecRunner` unit-tested with a fake binary on `PATH`.
- [ ] **Step 5:** Commit `feat(diags): runner and ibnetdiscover parser`.

### Task 4: PMA counter parsers (basic, -x, -T, optional)

**Files:** Create `internal/ib/diags/perfquery.go`, `testdata/perfquery/{basic,extended,speeds,rcverr,xmitdisc,congestion}.txt`; Test `perfquery_test.go`.

**Interfaces:**
- Produces: `type PortCounters struct{GUID string; Port int; Fields map[string]uint64}`, `func ParsePerfquery(out []byte) ([]PortCounters, error)` (parses `-l` multi-port output), `func MergeCounters(basic, ext []PortCounters) []PortCounters`, `func ToSamples(fabric string, node ib.Node, pc PortCounters) []ib.Sample` (applies ×4 to `PortXmitData/PortRcvData`, `_total` names per spec).
- Consumes: `Runner` from Task 3.

- [ ] **Step 1:** Failing tests: `TestParse_PerPortLoopOutput`, `TestMerge_BasicErrorsPlusExtendedData`, `TestToSamples_DataTimesFour`, `TestParse_NAValueIsAbsent`, `TestParse_SaturatedCounterKept`. `-T` field names are not in the spec: read `infiniband-diags/perfquery.c` in rdma-core, record the exact labels in `docs/metrics.md`, build fixtures from those labels.
- [ ] **Step 2–4:** FAIL → implement → PASS.
- [ ] **Step 5:** Commit `feat(diags): perfquery parsers`.

### Task 5: Port info, SM and optional ibswinfo parsers

**Files:** Create `internal/ib/diags/{portinfo.go,sm.go,ibswinfo.go}`, fixtures; Test alongside.

**Interfaces:**
- Produces: `ParsePortInfo([]byte) (PortInfo, error)` (state, phys state, width, speed, MTU), `ParseSMInfo([]byte) (SMInfo, error)` (guid, lid, priority, state, activity count), `ParseIbswinfo([]byte) (SwitchHealth, error)` (temperature, fans, PSUs, uptime, hardware info). Samples builders `PortInfoSamples`, `SMSamples`, `SwitchHealthSamples`.

- [ ] **Step 1:** Failing tests `TestParsePortInfo_ActiveLink`, `TestParsePortInfo_Down`, `TestParseSMInfo_Master`, `TestParseIbswinfo_Basic`, `TestParseIbswinfo_Garbage_Absent`.
- [ ] **Step 2–4:** FAIL → implement → PASS. `ibswinfo` stays off by default.
- [ ] **Step 5:** Commit `feat(diags): portinfo, sminfo, ibswinfo parsers`.

### Task 6: Local HCA sysfs reader and replay reader

**Files:** Create `internal/ib/sysfs/{hca_linux.go,hca_other.go}`, `internal/ib/replay/replay.go`, `testdata/sysfs/` tree; Test `hca_linux_test.go`, `replay_test.go`.

**Interfaces:**
- Produces: `func ReadHCA(root string) ([]ib.Sample, error)` (root injectable, default `/sys/class/infiniband`; reads `counters/`, `counters_ext/`, every file in `hw_counters/`, `state`, `phys_state`, `rate`, `link_layer`, `node_guid`, `lid`, `sm_lid`; emits `ib_hca_*`, `ib_hca_hw_counter_total{name}`, `ib_hca_info`). `replay.Runner` implements `diags.Runner` from a fixture directory.

- [ ] **Step 1:** Failing tests `TestReadHCA_EnumeratesAllHwCounters` (unknown file name appears), `TestReadHCA_NoDeviceNameAssumption` (device `foo_0` works), `TestReadHCA_UnreadableFileSkipped`, `TestReplayRunner_ReturnsFixture`.
- [ ] **Step 2–4:** FAIL → implement → PASS; `GOOS=darwin go build ./...` compiles.
- [ ] **Step 5:** Commit `feat(ib): sysfs HCA and replay readers`.

### Task 7: Collection loop

**Files:** Create `internal/ib/collector.go`; Test `collector_test.go`.

**Interfaces:**
- Consumes: Tasks 2–6.
- Produces: `func NewCollectionLoop(cfg *config.Config, r diags.Runner, store *SnapshotStore, opts ...Option) *Loop`, `func (l *Loop) Run(ctx context.Context)`, `func (l *Loop) Once(ctx context.Context) error`; `WithClock`, `WithHCARoot`.

- [ ] **Step 1:** Failing tests with the replay runner: `TestLoop_BuildsSnapshotPerFabric`, `TestLoop_ResolvesGUIDToCurrentLID` (LID changes between cycles), `TestLoop_OptionalReadersEveryNthCycle`, `TestLoop_FabricFailureSetsUpZeroOthersContinue`, `TestLoop_MaxConcurrentRespected`, `TestLoop_StaleLIDYieldsAbsentSamplesAndErrorCount`.
- [ ] **Step 2–4:** FAIL → implement (errgroup `SetLimit`, topology/portinfo on their own intervals, decimation counter) → PASS under `-race`.
- [ ] **Step 5:** Commit `feat(ib): collection loop`.

### Task 8: Prometheus and OTLP exports, metrics catalog, label invariant

**Files:** Create `internal/ib/{metrics.go,prometheus.go,otlp.go,buildinfo.go}`, `docs/metrics.md`; Test `export_test.go`.

**Interfaces:**
- Produces: `func NewSnapshotCollector(store *SnapshotStore) prometheus.Collector` (unchecked), `func NewOTLPBridge(store *SnapshotStore, meter metric.Meter) error`, `func NewBuildInfoCollector(version, goversion string) prometheus.Collector`, `func LabelsFor(...)` shared builders adding `fabric`.

- [ ] **Step 1:** Failing tests `TestPrometheus_AllSeriesCarryFabric`, `TestMetrics_LabelKeySetUniquePerName` (union invariant across switch/HCA/local paths), `TestOTLP_ManualReaderMatchesPrometheus`, `TestBuildInfo_Labels`, `TestUp_PerFabric`.
- [ ] **Step 2–4:** FAIL → implement → PASS. Document every metric in `docs/metrics.md` and add a `go test` that fails if a registered name is missing from it.
- [ ] **Step 5:** Commit `feat(ib): prometheus and otlp exports`.

### Task 9: Server, probes, main

**Files:** Create `internal/server/server.go`, `main.go`; Test `server_test.go`.

**Interfaces:**
- Produces: `func RegisterEndpoints(mux *http.ServeMux, cfg *config.Config, store *ib.SnapshotStore)` (routes registered from `config.ReservedURIs`, panics on length mismatch); flags `--config --debug --once --trace`; SIGHUP + fsnotify reload.

- [ ] **Step 1:** Failing tests on the real mux: `TestLivezReadyz_AlwaysOK`, `TestHealth_200EvenWhenFabricDown_BodyListsFabrics`, `TestRegisterEndpoints_ReservedMapDrift` (delete a registration → test fails), `TestOnceDebug_PrintsSortedSamples`.
- [ ] **Step 2–4:** FAIL → implement (HTTP up before first cycle) → PASS. Run once the "delete the route and confirm it fails" check manually.
- [ ] **Step 5:** Commit `feat: server, probes and CLI`.

### Task 10: Packaging, demo stack, dashboards

**Files:** Create `Dockerfile`, `Dockerfile.goreleaser`, `docker-compose.yml`, `docker-compose.ghcr.yml`, `prometheus.yml`, `deploy/prometheus/ib.rules.yml`, `grafana/provisioning/**`, dashboards JSON (overview, fabric errors, congestion/FEC, HCA node, SM/topology), `.env.example`.

- [ ] **Step 1:** Dockerfile on `debian:trixie-slim`: `apt-get install -y --no-install-recommends infiniband-diags curl`, user `ib` uid `10001`, `HEALTHCHECK` on `127.0.0.1:9449/livez`. Verify `apt-cache policy infiniband-diags` in the build log.
- [ ] **Step 2:** Compose runs with `replay.dir` and fixtures (no IB hardware); real-hardware override documents `--device /dev/infiniband` and `group_add: ["${UMAD_GID}"]`.
- [ ] **Step 3:** Alert rules: link downed, `ib_port_fec_uncorrectable_blocks_total` increase, SM change, `ib_up == 0`. Dashboards use `rate()` on `_total` counters, `fabric` template var.
- [ ] **Step 4:** `docker compose up -d` then `curl -s 127.0.0.1:9449/metrics | grep -c '^ib_'` > 0 and Grafana provisioning loads.
- [ ] **Step 5:** Commit `feat: packaging and demo stack`.

### Task 11: CI/CD, docs, ADRs

**Files:** Create `.github/workflows/{ci,security,release,docs}.yml` (thin callers from `fjacquet/ci/templates/workflows/`), `.github/dependabot.yml` (`gomod`, `docker`), `.goreleaser.yaml`, `mkdocs.yml`, `docs/{index,dashboards,deployment/docker}.md`, `docs/adr/0001..0009`, `CLAUDE.md`, `README.md` with badges.

- [ ] **Step 1:** ADRs per spec: snapshot model; dual export; client choice (exec + sysfs, no SDK/REST); counters as `_total`; no counter reset; GUID+port identity; label union; Debian slim base; UMAD access.
- [ ] **Step 2:** Add a drift-table line in `~/.claude/skills/exporter-standards/references/decisions.md` for the Debian base and `rate()` exception, and add `ib_exporter` (prefix `ib_`, port `9449`) to the family table in `SKILL.md`.
- [ ] **Step 3:** `make ci` green; `make release-snapshot` builds linux/darwin × amd64/arm64.
- [ ] **Step 4:** Commit `docs: ci, adrs and documentation`.

---

## Self-review

- **Spec coverage:** readers/cadence (T3–T7), config shape (T1), metrics catalog (T4–T8), fabric label + `ib_up` (T7–T8), probes/reserved URIs (T1, T9), packaging/UMAD/Debian (T10), ADRs/drift (T11), fixtures/replay (T3–T6), `--once --debug --trace` (T9). Phase 2 (`mlxlink`, BER/eye) and UFM/native MAD are non-goals and have no task.
- **Open on hardware:** `-T` field names (T4 reads `perfquery.c`), `-T` with `-l`, `-x` error content (design reads both), `ibswinfo` packaging, umad group. Each is marked in its task or in the spec.
- **Type consistency:** `Sample`, `Runner`, `SnapshotStore`, `FabricSnapshot` defined in T2/T3 and used unchanged afterward.
