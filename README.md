# DCM CLI Specification

## 1. Overview

### 1.1 Purpose

The DCM CLI (`dcm`) is the primary user-facing command-line interface for interacting with the DCM (Data Center Management) control plane. It provides commands for managing policies, service types, catalog items, catalog item instances, service provider resources, and environment agents through the control-plane monolith API.

### 1.2 Version Scope

This specification covers the `v1alpha1` API surface, matching the control-plane route prefix `/api/v1alpha1`.

### 1.3 Reference Documents

| Document | Description |
|----------|-------------|
| Control-plane OpenAPI | `api/*/v1alpha1/openapi.yaml` in dcm-project/control-plane |
| AEP Standards | [aep.dev](https://aep.dev/) - API Enhancement Proposals |
| RFC 7807 | Problem Details for HTTP APIs |
| RFC 7396 | JSON Merge Patch |
| RFC 8628 | OAuth 2.0 Device Authorization Grant |

---

## 2. Architecture

### 2.1 System Context

```
┌─────────┐               ┌────────────────────────────────────────────────────┐
│         │               │           control-plane monolith                   │
│  dcm    │─────────────▶│        (port 8080, /api/v1alpha1/*)                │
│  CLI    │ HTTP / HTTPS  │                                                    │
│         │  + Bearer JWT │  Policy Manager · Catalog Manager · SP Manager     │
│         │               │                                                    │
└─────────┘               └────────────────────────────────────────────────────┘
      │
      │  OIDC Discovery
      │  + Device Auth
      ▼
┌─────────────────┐
│    Keycloak      │
│  (realm: dcm)    │
│  client: dcm-cli │
└─────────────────┘
```

The CLI communicates exclusively through the control plane (port 8080). When the control plane URL uses an `https://` scheme, the CLI establishes a TLS connection. When the URL uses `http://`, TLS is skipped entirely. When authentication is configured, the CLI obtains tokens from a Keycloak OIDC provider and injects them as Bearer JWTs in API requests. These managers run in-process in the monolith (formerly separate services). CLI commands call paths under `/api/v1alpha1`:

- `/api/v1alpha1/policies/*` → Policy Manager
- `/api/v1alpha1/service-types/*` → Catalog Manager
- `/api/v1alpha1/catalog-items/*` → Catalog Manager
- `/api/v1alpha1/catalog-item-instances/*` → Catalog Manager
- `/api/v1alpha1/service-type-instances/*` → SP Resource Manager
- `/api/v1alpha1/agents/*` → Agent Manager

### 2.2 Internal Architecture

```
cmd/dcm/
  main.go                    ← Entry point, root command setup

internal/
  auth/                      ← OIDC authentication (device flow, token storage, transport)
  config/                    ← Configuration loading/saving
  output/                    ← Output formatting (table/json/yaml)
  commands/
    root.go                  ← Root command, global flags
    login.go                 ← OIDC device authorization login
    logout.go                ← Token revocation and credential cleanup
    version.go               ← Version command
    policy.go                ← Policy command group
    catalog_service_type.go  ← Catalog service-type command group
    catalog_item.go          ← Catalog item command group
    catalog_instance.go      ← Catalog instance command group
    sp.go                    ← SP parent command group
    sp_resource.go           ← SP resource command group
    agent.go                 ← Environment agent command group
    completion.go            ← Shell completion command
```

### 2.3 Component Descriptions

| Component | Responsibility |
|-----------|---------------|
| `cmd/dcm/main.go` | Bootstrap, wire dependencies, execute root command |
| `internal/auth` | OIDC device flow, token storage (keyring + file), authenticated transport |
| `internal/config` | Load config from file/env/flags with precedence |
| `internal/output` | Format responses as table, JSON, or YAML |
| `internal/commands` | Cobra command definitions, flag binding, client invocation |
| `internal/version` | Build-time version info injected via ldflags |

The CLI uses **generated clients** from the [control-plane](https://github.com/dcm-project/control-plane/tree/main/pkg) repo ([policy/client](https://github.com/dcm-project/control-plane/tree/main/pkg/policy/client), [catalog/client](https://github.com/dcm-project/control-plane/tree/main/pkg/catalog/client), [sp/client](https://github.com/dcm-project/control-plane/tree/main/pkg/sp/client)) (oapi-codegen generated) as Go module dependencies. No hand-written HTTP client is needed.

---

## 3. Configuration

### 3.1 Configuration File

Default location: `~/.dcm/config.yaml`

```yaml
control-plane-url: http://localhost:8080
issuer-url: ""
output-format: table
timeout: 30
tls-ca-cert: ""
tls-client-cert: ""
tls-client-key: ""
tls-skip-verify: false
```

Note: `dcm login` automatically creates and updates this file (see [Authentication](#13-authentication)).

### 3.2 Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `DCM_CONTROL_PLANE_URL` | Control plane API base URL | `http://localhost:8080` |
| `DCM_OUTPUT_FORMAT` | Output format (`table`, `json`, `yaml`) | `table` |
| `DCM_TIMEOUT` | Request timeout in seconds | `30` |
| `DCM_CONFIG` | Path to config file | `~/.dcm/config.yaml` |
| `DCM_ISSUER_URL` | OIDC issuer URL (Keycloak realm URL) | `""` |
| `DCM_TOKEN` | Static Bearer token for CI/scripting (bypasses device flow) | `""` |
| `DCM_TLS_CA_CERT` | Path to CA certificate file for TLS verification | `""` |
| `DCM_TLS_CLIENT_CERT` | Path to client certificate file for mTLS | `""` |
| `DCM_TLS_CLIENT_KEY` | Path to client private key file for mTLS | `""` |
| `DCM_TLS_SKIP_VERIFY` | Skip TLS certificate verification (`true`/`false`) | `false` |

### 3.3 Precedence Order

Configuration values are resolved in the following order (highest to lowest priority):

1. **Command-line flags** (`--control-plane-url`, `--output`, `--timeout`, `--issuer-url`, `--token`)
2. **Environment variables** (`DCM_CONTROL_PLANE_URL`, `DCM_ISSUER_URL`, `DCM_TOKEN`, etc.)
3. **Configuration file** (`~/.dcm/config.yaml`)
4. **Built-in defaults**

### 3.4 Global Flags

These flags are available on all commands:

| Flag | Short | Description |
|------|-------|-------------|
| `--control-plane-url` | | Control plane API URL |
| `--output` | `-o` | Output format: `table`, `json`, `yaml` |
| `--timeout` | | Request timeout in seconds |
| `--config` | | Path to config file |
| `--tls-ca-cert` | | Path to CA certificate file for TLS verification |
| `--tls-client-cert` | | Path to client certificate file for mTLS |
| `--tls-client-key` | | Path to client private key file for mTLS |
| `--issuer-url` | | OIDC issuer URL for authentication |
| `--token` | | Static Bearer token (CI/scripting) |
| `--tls-skip-verify` | | Skip TLS certificate verification |

---

## 4. Command Tree

### 4.1 Top-Level Structure

```
dcm
├── login           # OIDC device authorization login
├── logout          # Revoke tokens and clear credentials
├── policy          # Policy management
│   ├── create
│   ├── list
│   ├── get
│   ├── update
│   └── delete
├── catalog
│   ├── service-type    # Service type management
│   │   ├── list
│   │   └── get
│   ├── item            # Catalog item management
│   │   ├── create
│   │   ├── list
│   │   ├── get
│   │   └── delete
│   └── instance        # Catalog item instance management
│       ├── create
│       ├── list
│       ├── get
│       └── delete
├── sp
│   └── resource       # SP resource management (read-only)
│       ├── list
│       └── get
├── completion      # Shell autocompletion
└── version         # Print version info
```

### 4.2 Login Command

#### `dcm login`

Authenticate with the DCM control plane using the OIDC Device Authorization Grant (RFC 8628). Initiates a device flow, opens a browser for the user to authenticate, and stores the resulting tokens locally.

On success, `dcm login` saves `issuer-url` to the active config file (`--config` / `DCM_CONFIG`, or `~/.dcm/config.yaml`). It also saves `control-plane-url` when that value was explicitly provided via `--control-plane-url` or `DCM_CONTROL_PLANE_URL` (the built-in default alone is not written).

| Flag | Required | Description |
|------|----------|-------------|
| `--issuer-url` | Yes | OIDC issuer URL (e.g., Keycloak realm URL) |
| `--control-plane-url` | No | Control plane URL to persist to config |

```bash
# Login to a DCM instance
dcm login --issuer-url https://keycloak.example.com/realms/dcm --control-plane-url https://dcm.example.com

# After first login, issuer-url (and control-plane-url when set) are saved to config
dcm policy list
```

Example output:

```
Open https://keycloak.example.com/realms/dcm/device?user_code=ABCD-EFGH in your browser
Or visit https://keycloak.example.com/realms/dcm/device and enter code: ABCD-EFGH
Logged in as dcm-admin (token expires in 5m0s; auto-refresh enabled)
```

Tokens are stored in the OS keychain (macOS Keychain, Linux Secret Service, Windows Credential Manager) when available, falling back to `~/.dcm/tokens.json` (mode `0600`) in environments without keychain support (containers, CI, headless SSH).

### 4.3 Logout Command

#### `dcm logout`

Revoke stored tokens and clear authentication credentials.

| Flag | Required | Description |
|------|----------|-------------|
| `--issuer-url` | Yes | OIDC issuer URL used during login |

```bash
dcm logout --issuer-url https://keycloak.example.com/realms/dcm
```

Example output:

```
Logged out successfully
```

If no stored credentials are found, the command prints "No stored credentials found" and exits successfully. Token revocation failures produce a warning but do not cause the command to fail.

### 4.5 Policy Commands

#### `dcm policy create`

Create a new policy.

| Flag | Required | Description |
|------|----------|-------------|
| `--from-file` | Yes | Path to policy YAML/JSON file |
| `--id` | No | Client-specified policy ID (DNS-1123 format) |

```bash
# Create a policy from file
dcm policy create --from-file policy.yaml

# Create with client-specified ID
dcm policy create --from-file policy.yaml --id my-policy
```

Policy file format:

```yaml
display_name: "Require CPU Limits"
description: "Ensures all containers have CPU limits set"
policy_type: GLOBAL
label_selector:
  environment: production
priority: 100
rego_code: |
  package dcm.policy
  default allow = false
  allow { input.request.resource.limits.cpu != "" }
enabled: true
```

Example output (table):

```
ID          DISPLAY NAME        TYPE    PRIORITY  ENABLED  CREATED
my-policy   Require CPU Limits  GLOBAL  100       true     2026-03-09T10:00:00Z
```

#### `dcm policy list`

List policies with optional filtering and ordering.

| Flag | Required | Description |
|------|----------|-------------|
| `--filter` | No | CEL filter expression (e.g., `policy_type='GLOBAL'`) |
| `--order-by` | No | Order field and direction (e.g., `priority asc`) |
| `--page-size` | No | Maximum results per page |
| `--page-token` | No | Token for next page |

```bash
# List all policies
dcm policy list

# List enabled global policies ordered by priority
dcm policy list --filter "policy_type='GLOBAL' AND enabled=true" --order-by "priority asc"

# List with pagination
dcm policy list --page-size 10

# JSON output
dcm policy list -o json
```

Example output (table):

```
ID          DISPLAY NAME        TYPE    PRIORITY  ENABLED  CREATED
my-policy   Require CPU Limits  GLOBAL  100       true     2026-03-09T10:00:00Z
other-pol   Memory Limits       USER    500       true     2026-03-08T15:30:00Z
```

#### `dcm policy get`

Get a single policy by ID.

| Argument | Required | Description |
|----------|----------|-------------|
| `POLICY_ID` | Yes | Policy ID |

```bash
dcm policy get POLICY_ID

# YAML output
dcm policy get POLICY_ID -o yaml
```

#### `dcm policy update`

Update an existing policy (JSON Merge Patch - RFC 7396).

| Argument | Required | Description |
|----------|----------|-------------|
| `POLICY_ID` | Yes | Policy ID |

| Flag | Required | Description |
|------|----------|-------------|
| `--from-file` | Yes | Path to patch YAML/JSON file |

```bash
dcm policy update POLICY_ID --from-file patch.yaml
```

Patch file (only mutable fields):

```yaml
priority: 50
enabled: false
rego_code: |
  package dcm.policy
  default allow = true
```

#### `dcm policy delete`

Delete a policy by ID.

| Argument | Required | Description |
|----------|----------|-------------|
| `POLICY_ID` | Yes | Policy ID |

```bash
dcm policy delete POLICY_ID
```

Example output:

```
Policy "my-policy" deleted successfully.
```

### 4.6 Catalog Service-Type Commands

#### `dcm catalog service-type list`

List service types with optional pagination.

| Flag | Required | Description |
|------|----------|-------------|
| `--page-size` | No | Maximum results per page |
| `--page-token` | No | Token for next page |

```bash
dcm catalog service-type list
dcm catalog service-type list --page-size 5
```

#### `dcm catalog service-type get`

Get a single service type by ID.

| Argument | Required | Description |
|----------|----------|-------------|
| `SERVICE_TYPE_ID` | Yes | Service type ID |

```bash
dcm catalog service-type get SERVICE_TYPE_ID
```

### 4.7 Catalog Item Commands

#### `dcm catalog item create`

Create a new catalog item.

| Flag | Required | Description |
|------|----------|-------------|
| `--from-file` | Yes | Path to catalog item YAML/JSON file |
| `--id` | No | Client-specified catalog item ID |

```bash
dcm catalog item create --from-file item.yaml
dcm catalog item create --from-file item.yaml --id my-catalog-item
```

Catalog item file format (multi-resource schema):

```yaml
api_version: v1alpha1
display_name: "App with Database"
spec:
  resources:
    - name: app
      service_type: container
      fields:
        - path: image.reference
          display_name: "Container Image"
          editable: true
        - path: resources.cpu.min
          display_name: "CPU Min"
          editable: true
          default: "1"
          validation_schema:
            type: string
            pattern: '^[1-9][0-9]*m?$'
    - name: db
      service_type: database
      requires_resources:
        - app
      fields:
        - path: engine
          editable: false
          default: "postgres"
        - path: version
          editable: false
          default: "16"
```

Example output (table):

```
UID                                   DISPLAY NAME        CREATED
b2c3d4e5-f6a7-8901-bcde-f12345678901  App with Database   2026-03-09T10:00:00Z
```

#### `dcm catalog item list`

List catalog items with optional filtering and pagination.

| Flag | Required | Description |
|------|----------|-------------|
| `--service-type` | No | Filter by service type |
| `--page-size` | No | Maximum results per page |
| `--page-token` | No | Token for next page |

```bash
dcm catalog item list
dcm catalog item list --service-type container
```

#### `dcm catalog item get`

Get a single catalog item by ID.

| Argument | Required | Description |
|----------|----------|-------------|
| `CATALOG_ITEM_ID` | Yes | Catalog item ID |

```bash
dcm catalog item get CATALOG_ITEM_ID
dcm catalog item get CATALOG_ITEM_ID -o yaml
```

#### `dcm catalog item delete`

Delete a catalog item by ID.

| Argument | Required | Description |
|----------|----------|-------------|
| `CATALOG_ITEM_ID` | Yes | Catalog item ID |

```bash
dcm catalog item delete CATALOG_ITEM_ID
```

### 4.8 Catalog Instance Commands

#### `dcm catalog instance create`

Create a new catalog item instance.

| Flag | Required | Description |
|------|----------|-------------|
| `--from-file` | Yes | Path to instance YAML/JSON file |
| `--id` | No | Client-specified instance ID |
| `--generate-ssh-key` | No | Local account name. Generates an SSH key pair on this machine and sends only the public key |
| `--ssh-key-file` | No | Where to write the generated private key (default `<login>_ed25519`); the public key goes beside it as `.pub` |
| `--ssh-key-resource` | No | Catalog item resource that takes the key (default: the only resource in the file) |
| `--ssh-key-path` | No | User value path of the authorized keys list (default `authorized_keys`) |

```bash
dcm catalog instance create --from-file instance.yaml
```

With `--generate-ssh-key`, the key pair is made by the CLI. The request gains one entry,
`{login, public_key}`, in the resource's `authorized_keys` user value (UDLM Machine `authorized_keys`).
The private key is written to a file with mode 0600 and is not sent or stored anywhere else. An
existing key file is never replaced, and the files are removed if the control plane refuses the
request.

```bash
dcm catalog instance create --from-file instance.yaml --generate-ssh-key cloud-user --ssh-key-file ./bastion-01
ssh -i ./bastion-01 cloud-user@<address>
```

Instance file format (multi-resource schema):

```yaml
api_version: v1alpha1
display_name: "My Dev VM"
spec:
  catalog_item_id: small-vm
  user_values:
    - resource: app
      path: image.reference
      value: "nginx:latest"
    - resource: app
      path: resources.cpu.min
      value: "2"
```

Example output (table):

```
ID            UID                                   DISPLAY NAME      CATALOG ITEM      CREATED
my-instance   c3d4e5f6-a7b8-9012-cdef-123456789012  My App Instance   my-catalog-item   2026-03-09T10:00:00Z
```

#### `dcm catalog instance list`

List catalog item instances with optional pagination.

| Flag | Required | Description |
|------|----------|-------------|
| `--page-size` | No | Maximum results per page |
| `--page-token` | No | Token for next page |

```bash
dcm catalog instance list
dcm catalog instance list -o json
```

#### `dcm catalog instance get`

Get a single catalog item instance by ID.

| Argument | Required | Description |
|----------|----------|-------------|
| `INSTANCE_ID` | Yes | Instance ID |

```bash
dcm catalog instance get INSTANCE_ID
```

#### `dcm catalog instance delete`

Delete a catalog item instance by ID.

| Argument | Required | Description |
|----------|----------|-------------|
| `INSTANCE_ID` | Yes | Instance ID |

```bash
dcm catalog instance delete INSTANCE_ID
```

### 4.9 SP Resource Commands

#### `dcm sp resource list`

List SP resources (service type instances) with optional filtering and pagination.

| Flag | Required | Description |
|------|----------|-------------|
| `--agent-name` | No | Filter by agent name |
| `--page-size` | No | Maximum results per page |
| `--page-token` | No | Token for next page |
| `--show-deleted` | No | Include soft-deleted resources |

```bash
dcm sp resource list
dcm sp resource list --agent-name kubevirt-east
dcm sp resource list --page-size 5
```

Example output (table):

```
ID              AGENT           STATUS   CREATED
my-instance     kubevirt-east   ACTIVE   2026-03-09T10:00:00Z
other-instance  openstack-west  PENDING  2026-03-08T15:30:00Z
```

#### `dcm sp resource get`

Get a single SP resource by instance ID.

| Argument | Required | Description |
|----------|----------|-------------|
| `INSTANCE_ID` | Yes | Service type instance ID |

```bash
dcm sp resource get INSTANCE_ID
dcm sp resource get INSTANCE_ID -o yaml
```

### 4.10 Agent Commands

#### `dcm agent list`

List environment agents with optional filtering and pagination.

| Flag | Required | Description |
|------|----------|-------------|
| `--health-status` | No | Filter by health status (`ready`, `congested`, `unavailable`) |
| `--page-size` | No | Maximum results per page |
| `--page-token` | No | Token for next page |

```bash
dcm agent list
dcm agent list --health-status ready
dcm agent list --page-size 5
```

Example output (table):

```
ID         NAME            ENVIRONMENT  HEALTH  CREATED
agent-123  kubevirt-east   production   ready   2026-03-09T10:00:00Z
```

#### `dcm agent get`

Get a single environment agent by ID.

| Argument | Required | Description |
|----------|----------|-------------|
| `AGENT_ID` | Yes | Agent ID |

```bash
dcm agent get AGENT_ID
dcm agent get AGENT_ID -o yaml
```

### 4.11 Completion Command

#### `dcm completion`

Generate shell autocompletion scripts.

| Argument | Required | Description |
|----------|----------|-------------|
| `SHELL` | Yes | Shell type: `bash`, `zsh`, `fish`, or `powershell` |

```bash
# Bash
source <(dcm completion bash)

# Zsh
dcm completion zsh > "${fpath[1]}/_dcm"

# Fish
dcm completion fish | source

# PowerShell
dcm completion powershell | Out-String | Invoke-Expression
```

### 4.12 Version Command

#### `dcm version`

Print CLI version and build information.

```bash
dcm version
```

Example output:

```
dcm version 0.1.0
  commit: abc1234
  built:  2026-03-09T10:00:00Z
  go:     go1.25.5
```

---

## 5. Internal Components

### 5.1 `internal/config`

Manages CLI configuration with file persistence and environment/flag overrides.

```go
package config

type Config struct {
    ControlPlaneURL string `yaml:"control-plane-url" mapstructure:"control-plane-url"`
    IssuerURL     string `yaml:"issuer-url" mapstructure:"issuer-url"`
    Token         string `yaml:"-" mapstructure:"token"`
    OutputFormat  string `yaml:"output-format" mapstructure:"output-format"`
    Timeout       int    `yaml:"timeout" mapstructure:"timeout"`
    TLSCACert     string `yaml:"tls-ca-cert" mapstructure:"tls-ca-cert"`
    TLSClientCert string `yaml:"tls-client-cert" mapstructure:"tls-client-cert"`
    TLSClientKey  string `yaml:"tls-client-key" mapstructure:"tls-client-key"`
    TLSSkipVerify bool   `yaml:"tls-skip-verify" mapstructure:"tls-skip-verify"`
}

// Load reads configuration from file, environment, and flag overrides.
func Load() (*Config, error)

// ConfigPath returns the resolved config file path for cmd.
func ConfigPath(cmd *cobra.Command) string

// SaveConfig merges values into the config file at path (atomic write).
// Empty path writes to ~/.dcm/config.yaml.
func SaveConfig(path string, values map[string]string) error

```

Note: `Token` has `yaml:"-"` - it is never persisted to the config file for security. It is only available via the `DCM_TOKEN` env var or `--token` flag.

### 5.2 `internal/output`

Formats API responses for display.

```go
package output

type Format string

const (
    FormatTable Format = "table"
    FormatJSON  Format = "json"
    FormatYAML  Format = "yaml"
)

type Formatter interface {
    // FormatOne formats a single resource.
    FormatOne(resource any) error
    // FormatList formats a list of resources with optional pagination info.
    FormatList(resources any, nextPageToken string) error
    // FormatMessage formats a simple status message.
    FormatMessage(msg string) error
}

// New creates a Formatter for the given format writing to the given writer.
func New(format Format, w io.Writer) Formatter
```

### 5.3 `internal/commands`

Cobra command definitions. Each file registers its command tree and wires generated client calls.

```go
// root.go
func NewRootCommand() *cobra.Command

// login.go
func newLoginCommand() *cobra.Command   // dcm login

// logout.go
func newLogoutCommand() *cobra.Command  // dcm logout

// policy.go
func newPolicyCommand() *cobra.Command       // parent: dcm policy
func newPolicyCreateCommand() *cobra.Command  // dcm policy create
func newPolicyListCommand() *cobra.Command    // dcm policy list
func newPolicyGetCommand() *cobra.Command     // dcm policy get
func newPolicyUpdateCommand() *cobra.Command  // dcm policy update
func newPolicyDeleteCommand() *cobra.Command  // dcm policy delete

// catalog_service_type.go
func newCatalogServiceTypeCommand() *cobra.Command       // parent: dcm catalog service-type
func newServiceTypeListCommand() *cobra.Command
func newServiceTypeGetCommand() *cobra.Command

// catalog_item.go
func newCatalogItemCommand() *cobra.Command              // parent: dcm catalog item
func newCatalogItemCreateCommand() *cobra.Command
func newCatalogItemListCommand() *cobra.Command
func newCatalogItemGetCommand() *cobra.Command
func newCatalogItemDeleteCommand() *cobra.Command

// catalog_instance.go
func newCatalogInstanceCommand() *cobra.Command           // parent: dcm catalog instance
func newCatalogInstanceCreateCommand() *cobra.Command
func newCatalogInstanceListCommand() *cobra.Command
func newCatalogInstanceGetCommand() *cobra.Command
func newCatalogInstanceDeleteCommand() *cobra.Command

// sp.go
func newSPCommand() *cobra.Command                    // parent: dcm sp

// sp_resource.go
func newSPResourceCommand() *cobra.Command             // parent: dcm sp resource
func newSPResourceListCommand() *cobra.Command
func newSPResourceGetCommand() *cobra.Command

// completion.go
func newCompletionCommand() *cobra.Command              // dcm completion [bash|zsh|fish|powershell]
```

### 5.4 `internal/auth`

OIDC device authorization, token storage (keyring with file fallback), and `AuthTransport` for Bearer injection/refresh. Public entry points: `DeviceLogin`, `RevokeToken`, `NewTokenStore`, `AuthTransport`. Behavior and usage are covered in [§13 Authentication](#13-authentication).

### 5.5 `internal/version`

Build-time version information injected via linker flags.

```go
package version

var (
    Version   = "dev"
    Commit    = "unknown"
    BuildTime = "unknown"
)

type Info struct {
    Version   string
    Commit    string
    BuildTime string
    GoVersion string
}

func Get() Info
```

### 5.6 Generated Clients (External Dependencies)

The CLI imports generated client packages from the control-plane monorepo:

- [pkg/policy/client](https://github.com/dcm-project/control-plane/tree/main/pkg/policy/client) — Policy Manager client
- [pkg/catalog/client](https://github.com/dcm-project/control-plane/tree/main/pkg/catalog/client) — Catalog Manager client
- [pkg/sp/client/resource_manager](https://github.com/dcm-project/control-plane/tree/main/pkg/sp/client/resource_manager) — SP Resource Manager client
- [pkg/agent/client](https://github.com/dcm-project/control-plane/tree/main/pkg/agent/client) — Agent Manager client

These are oapi-codegen generated clients providing typed API access. Key interfaces:

```go
// Policy client (from control-plane/pkg/policy/client)
type ClientInterface interface {
    CreatePolicy(ctx context.Context, params *CreatePolicyParams, body CreatePolicyJSONRequestBody, ...) (*http.Response, error)
    ListPolicies(ctx context.Context, params *ListPoliciesParams, ...) (*http.Response, error)
    GetPolicy(ctx context.Context, policyId string, ...) (*http.Response, error)
    UpdatePolicyWithApplicationMergePatchPlusJSONBody(ctx context.Context, policyId string, body UpdatePolicyApplicationMergePatchPlusJSONRequestBody, ...) (*http.Response, error)
    DeletePolicy(ctx context.Context, policyId string, ...) (*http.Response, error)
}

// Catalog client (from control-plane/pkg/catalog/client)
type ClientInterface interface {
    CreateServiceType(ctx context.Context, params *CreateServiceTypeParams, body CreateServiceTypeJSONRequestBody, ...) (*http.Response, error)
    ListServiceTypes(ctx context.Context, params *ListServiceTypesParams, ...) (*http.Response, error)
    GetServiceType(ctx context.Context, serviceTypeId string, ...) (*http.Response, error)
    // ... similar for CatalogItem and CatalogItemInstance operations
}
```

Clients are instantiated with the control-plane URL and a configured HTTP client. When the control-plane URL uses `https://`, the HTTP client is configured with a TLS transport based on the TLS settings (CA cert, client cert/key, skip verify). When the URL uses `http://`, TLS is not configured. When authentication is configured (`issuer-url` or `token`), the base transport is wrapped with `AuthTransport` which lazily injects Bearer tokens. Token refresh uses a separate transport derived from the issuer URL (falling back to the control-plane base transport), so OIDC refresh works when the control plane is HTTP and the issuer is HTTPS with a private CA. Login and logout use a plain HTTP client (no AuthTransport) with TLS derived from the issuer URL for the same reason.

```go
httpClient := buildHTTPClient(cfg) // TLS + optional AuthTransport wrapping
policyClient, _ := policyclient.NewClient(cfg.ControlPlaneURL + "/api/v1alpha1",
    policyclient.WithHTTPClient(httpClient))
catalogClient, _ := catalogclient.NewClient(cfg.ControlPlaneURL + "/api/v1alpha1",
    catalogclient.WithHTTPClient(httpClient))
sprmClient, _ := sprmclient.NewClient(cfg.ControlPlaneURL + "/api/v1alpha1",
    sprmclient.WithHTTPClient(httpClient))
```

---

## 6. Project Structure

```
dcm-cli/
├── .ai/
│   └── specs/
│       └── dcm-cli.spec.md
├── cmd/
│   └── dcm/
│       └── main.go
├── internal/
│   ├── auth/
│   │   ├── auth.go
│   │   ├── auth_suite_test.go
│   │   ├── auth_test.go
│   │   ├── token.go
│   │   ├── token_test.go
│   │   ├── transport.go
│   │   └── transport_test.go
│   ├── config/
│   │   ├── config.go
│   │   └── config_test.go
│   ├── output/
│   │   ├── formatter.go
│   │   ├── table.go
│   │   ├── json.go
│   │   ├── yaml.go
│   │   └── formatter_test.go
│   ├── commands/
│   │   ├── root.go
│   │   ├── root_test.go
│   │   ├── login.go
│   │   ├── logout.go
│   │   ├── version.go
│   │   ├── policy.go
│   │   ├── policy_test.go
│   │   ├── catalog_service_type.go
│   │   ├── catalog_service_type_test.go
│   │   ├── catalog_item.go
│   │   ├── catalog_item_test.go
│   │   ├── catalog_instance.go
│   │   ├── catalog_instance_test.go
│   │   ├── sp.go
│   │   ├── sp_resource.go
│   │   ├── sp_resource_test.go
│   │   ├── completion.go
│   │   └── completion_test.go
│   └── version/
│       └── version.go
├── test/
│   └── e2e/
│       ├── policy_test.go
│       ├── catalog_test.go
│       └── e2e_suite_test.go
├── .github/
│   └── workflows/
│       ├── ci.yaml
│       ├── lint.yaml
│       ├── check-clean-commits.yaml
│       └── release.yaml
├── .goreleaser.yaml
├── CLAUDE.md
├── Containerfile
├── go.mod
├── go.sum
├── LICENSE
├── Makefile
├── README.md
└── tools.go
```

---

## 7. Operational Flows

### 7.1 Command Execution Flow

```
User invokes command
  │
  ├─▶ Cobra parses flags and arguments
  │
  ├─▶ Viper resolves config (flags → env → file → defaults)
  │
  ├─▶ Build HTTP client
  │     ├─ Configure TLS transport if URL is https://
  │     └─ Wrap with AuthTransport if issuer-url or token is set
  │
  ├─▶ Create generated client with control-plane URL and HTTP client
  │
  ├─▶ Execute API call via generated client
  │     │  (AuthTransport lazily injects Bearer token on first request)
  │     ├─ Valid cached token → inject, proceed
  │     ├─ Expired token → refresh, inject, proceed
  │     ├─ Static token (DCM_TOKEN) → inject directly, no refresh
  │     └─ No auth configured → no Authorization header
  │
  ├─▶ Check response status
  │     ├─ Success → format and display response
  │     └─ Error → parse RFC 7807 problem details, display error, set exit code
  │
  └─▶ Exit
```

### 7.2 Policy CRUD Flow

```
dcm policy create --from-file policy.yaml
  │
  ├─▶ Read and parse policy.yaml (YAML or JSON)
  ├─▶ POST /api/v1alpha1/policies (with optional ?id=<id>)
  ├─▶ Display created policy
  └─▶ Exit 0

dcm policy list --filter "enabled=true" --order-by "priority asc"
  │
  ├─▶ GET /api/v1alpha1/policies?filter=enabled%3Dtrue&order_by=priority+asc
  ├─▶ Display policy table
  ├─▶ If next_page_token present, show pagination hint
  └─▶ Exit 0

dcm policy get <id>
  │
  ├─▶ GET /api/v1alpha1/policies/<id>
  ├─▶ Display policy details
  └─▶ Exit 0

dcm policy update <id> --from-file patch.yaml
  │
  ├─▶ Read and parse patch.yaml
  ├─▶ PATCH /api/v1alpha1/policies/<id>
  ├─▶ Display updated policy
  └─▶ Exit 0

dcm policy delete <id>
  │
  ├─▶ DELETE /api/v1alpha1/policies/<id>
  ├─▶ Display success message
  └─▶ Exit 0
```

### 7.3 Authentication Flow

```
dcm login --issuer-url https://keycloak.example.com/realms/dcm --control-plane-url https://dcm.example.com
  │
  ├─▶ OIDC Discovery: GET <issuer-url>/.well-known/openid-configuration
  ├─▶ Device Auth:    POST <device_authorization_endpoint>
  ├─▶ Print verification URL and user code to stderr
  ├─▶ Open browser to verification_uri_complete (best-effort)
  ├─▶ Poll token endpoint until user completes browser auth
  ├─▶ Store tokens (access + refresh + ID + expiry + token endpoint)
  │     ├─ OS keychain (macOS Keychain, Linux Secret Service, Windows Credential Manager)
  │     └─ File fallback: ~/.dcm/tokens.json (mode 0600)
  ├─▶ Save issuer-url (and control-plane-url when explicitly set) to ~/.dcm/config.yaml
  ├─▶ Print "Logged in as <username> (token expires in <ttl>; auto-refresh enabled)"
  └─▶ Exit 0

dcm logout --issuer-url https://keycloak.example.com/realms/dcm
  │
  ├─▶ Load stored token for issuer URL
  ├─▶ POST revocation endpoint with refresh token (warning on failure)
  ├─▶ Delete stored token
  ├─▶ Print "Logged out successfully"
  └─▶ Exit 0
```

**CI/scripting path** (no interactive login):

```bash
# Obtain token externally (e.g., via client_credentials grant using dcm-proxy client)
export DCM_TOKEN="<bearer-token>"
export DCM_CONTROL_PLANE_URL="https://dcm.example.com"

# All commands use the static token directly - no refresh, no keychain
dcm policy list
```

### 7.4 Catalog Ordering Flow

```
dcm catalog instance create --from-file instance.yaml
  │
  ├─▶ Read and parse instance.yaml
  │     Contains: catalog_item_id, user_values (each with resource, path, value)
  ├─▶ POST /api/v1alpha1/catalog-item-instances
  ├─▶ Display created instance
  └─▶ Exit 0
```

### 7.5 Pagination

#### 7.5.1 Page Size

While `--page-size` is an optional parameter, services may impose a default value. Always check if
the response included `next_page_token`

#### 7.5.2 Next Page Token

When a list response includes `next_page_token`, the CLI displays it for manual follow-up:

```
dcm policy list --page-size 2
```

Output:

```
ID          DISPLAY NAME        TYPE    PRIORITY  ENABLED  CREATED
my-policy   Require CPU Limits  GLOBAL  100       true     2026-03-09T10:00:00Z
other-pol   Memory Limits       USER    500       true     2026-03-08T15:30:00Z

Next page: dcm policy list --page-size 2 --page-token eyJvZmZzZXQiOjJ9
```

For JSON/YAML output, `next_page_token` is included in the response object.

---

## 8. TLS Configuration

### 8.1 Protocol-Based TLS Behavior

TLS is determined automatically by the control-plane URL scheme:

- **`http://`** — TLS is not used. All TLS-related flags and config are ignored.
- **`https://`** — TLS is enabled. The CLI constructs a `tls.Config` and uses it for the HTTP transport.

### 8.2 TLS Options

| Option | Description |
|--------|-------------|
| `--tls-ca-cert` | Path to a PEM-encoded CA certificate file. Used to verify the server's certificate. When not set, the system's default CA bundle is used. |
| `--tls-client-cert` | Path to a PEM-encoded client certificate file for mutual TLS (mTLS). Must be used together with `--tls-client-key`. |
| `--tls-client-key` | Path to a PEM-encoded client private key file for mTLS. Must be used together with `--tls-client-cert`. |
| `--tls-skip-verify` | When set, the CLI skips server certificate verification. **Not recommended for production.** |

### 8.3 Validation Rules

- If `--tls-client-cert` is set, `--tls-client-key` MUST also be set (and vice versa). The CLI MUST exit with code 2 if only one is provided.
- TLS flags are silently ignored when the control-plane URL uses `http://`.
- If the CA cert, client cert, or client key file does not exist or is not readable, the CLI MUST exit with code 1 with a clear error message.

### 8.4 Configuration Precedence

TLS options follow the same precedence as other configuration:

1. CLI flags (`--tls-ca-cert`, `--tls-client-cert`, `--tls-client-key`, `--tls-skip-verify`)
2. Environment variables (`DCM_TLS_CA_CERT`, `DCM_TLS_CLIENT_CERT`, `DCM_TLS_CLIENT_KEY`, `DCM_TLS_SKIP_VERIFY`)
3. Configuration file (`tls-ca-cert`, `tls-client-cert`, `tls-client-key`, `tls-skip-verify`)
4. Built-in defaults (empty strings, `false`)

---

## 9. Error Handling

### 9.1 Error Categories

| Category | Description | Exit Code |
|----------|-------------|-----------|
| Configuration error | Invalid config file, missing required config | 1 |
| Connection error | Cannot reach control plane | 1 |
| API error | Backend returned an error response | 1 |
| Input error | Invalid flags, missing arguments, bad file | 2 |
| Timeout | Request exceeded timeout | 1 |

### 9.2 API Error Display

API errors follow RFC 7807 Problem Details format. The CLI parses the response and displays a human-readable error:

```
Error: NOT_FOUND - Policy "nonexistent" not found.
  Status: 404
  Detail: The requested policy resource does not exist.
```

For JSON/YAML output, the full Problem Details object is printed:

```json
{
  "type": "NOT_FOUND",
  "status": 404,
  "title": "Policy \"nonexistent\" not found.",
  "detail": "The requested policy resource does not exist."
}
```

### 9.3 Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | Runtime error (API error, connection error, timeout) |
| 2 | Usage error (invalid flags, missing arguments) |

---

## 10. Build and Distribution

### 10.1 Makefile Targets

```makefile
build          # Build dcm binary to bin/dcm
test           # Run unit tests with Ginkgo
test-e2e       # Run E2E tests (requires live stack)
fmt            # Format Go code
vet            # Run go vet
lint           # Run linter
clean          # Remove build artifacts
tidy           # Run go mod tidy
```

### 10.2 Version Injection

Version information is injected at build time via ldflags:

```makefile
VERSION ?= $(shell git describe --tags --always --dirty)
COMMIT  ?= $(shell git rev-parse --short HEAD)
BUILD_TIME ?= $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')

LDFLAGS = -X github.com/dcm-project/dcm-cli/internal/version.Version=$(VERSION) \
          -X github.com/dcm-project/dcm-cli/internal/version.Commit=$(COMMIT) \
          -X github.com/dcm-project/dcm-cli/internal/version.BuildTime=$(BUILD_TIME)

build:
	go build -ldflags "$(LDFLAGS)" -o bin/dcm ./cmd/dcm
```

### 10.3 Release

Releases are automated via GoReleaser and GitHub Actions through a single `release.yaml` workflow.

**Rolling pre-releases (push to main):**

Every push to main builds snapshot binaries and publishes two pre-releases:
- `main` — a rolling release replaced on every push (like the `:main` container image tag)
- `<sha7>` — an immutable release for pinning to a specific commit

**Formal releases (tag push):**

When a `v*` tag is pushed, the workflow runs CI and lint checks, then GoReleaser builds release binaries and creates a GitHub release with archives and checksums.

To create a release:

```bash
git tag v0.1.0
git push origin v0.1.0
```

### 10.4 Containerfile

Multi-stage UBI9 build matching other DCM services:

```dockerfile
# Build stage
FROM registry.access.redhat.com/ubi9/go-toolset:1.25.5 AS builder
WORKDIR /opt/app-root/src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o dcm ./cmd/dcm

# Runtime stage
FROM registry.access.redhat.com/ubi9/ubi-minimal:latest
COPY --from=builder /opt/app-root/src/dcm /usr/local/bin/dcm
USER 1001
ENTRYPOINT ["dcm"]
```

### 10.5 Go Module

```
module github.com/dcm-project/dcm-cli

go 1.25.5
```

### 10.6 Dependencies

| Dependency | Purpose |
|------------|---------|
| `github.com/spf13/cobra` | CLI framework |
| `github.com/spf13/viper` | Configuration management |
| `gopkg.in/yaml.v3` | YAML parsing/output |
| `github.com/dcm-project/control-plane` | Generated API clients (policy, catalog, SP) |
| `github.com/coreos/go-oidc/v3` | OIDC discovery and provider metadata |
| `golang.org/x/oauth2` | OAuth 2.0 device authorization flow, token refresh |
| `github.com/zalando/go-keyring` | OS keychain access (macOS, Linux, Windows) |
| `github.com/onsi/ginkgo/v2` | Test framework (test dependency) |
| `github.com/onsi/gomega` | Test matchers (test dependency) |

### 10.7 tools.go

```go
//go:build tools

package tools

import (
	_ "github.com/onsi/ginkgo/v2/ginkgo"
)
```

---

## 11. Testing Strategy

### 11.1 Unit Tests

- **Framework**: Ginkgo + Gomega
- **Location**: `*_test.go` files alongside source
- **Mocking**: `net/http/httptest` for HTTP-level mocking of generated client calls
- **Coverage areas**:
  - Configuration loading and precedence
  - Output formatting (table, JSON, YAML)
  - Command flag parsing and validation
  - API response handling and error parsing
  - Input file parsing (YAML/JSON)
  - OIDC device flow with mock OIDC server
  - Token storage (keyring, file fallback, atomic writes, expiry)
  - Authenticated transport (static token, stored token, refresh, passthrough)

Example test pattern:

```go
var _ = Describe("Policy Commands", func() {
    var (
        server *httptest.Server
        // ...
    )

    BeforeEach(func() {
        server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            // Mock API responses
        }))
    })

    AfterEach(func() {
        server.Close()
    })

    Describe("policy create", func() {
        It("should create a policy from a YAML file", func() {
            // ...
        })
    })
})
```

Running tests:

```bash
make test                          # All unit tests
go test -run TestName ./internal/commands  # Specific test
```

### 11.2 E2E Tests

- **Location**: `test/e2e/`
- **Build tag**: `//go:build e2e` (excluded from `make test`)
- **Requirements**: Live DCM stack (control-plane on :8080)
- **Framework**: Ginkgo + Gomega
- **Scope**: Full command execution against real services

Running E2E tests:

```bash
make test-e2e   # Requires DCM_CONTROL_PLANE_URL pointing to live stack
```

---

## 12. Scope Boundaries

### 12.1 In Scope (v1alpha1)

- Policy CRUD operations (create, list, get, update, delete)
- Service type read operations (list, get)
- Catalog item operations (create, list, get, delete)
- Catalog item instance operations (create, list, get, delete)
- SP resource read operations (list, get)
- Version display
- Output formatting (table, JSON, YAML)
- Configuration via file, environment variables, and flags
- Pagination support for list operations
- TLS support with custom CA certificates, client certificates (mTLS), and skip-verify
- Shell autocompletion generation (bash, zsh, fish, powershell)
- Container image for distribution
- OIDC authentication via Device Authorization Grant (RFC 8628)
- Token storage with OS keychain and file fallback
- Static Bearer token injection for CI/scripting (`DCM_TOKEN`)

### 12.2 Out of Scope (v1alpha1)

- Interactive/wizard-style resource creation
- Watch/streaming operations
- Plugin/extension system
- Offline mode or local caching
- Bulk operations
- Resource diff/dry-run
- Health check command for control-plane connectivity

---

## 13. Authentication

### 13.1 Overview

The CLI supports OIDC authentication via the OAuth 2.0 Device Authorization Grant (RFC 8628). When authentication is configured, API requests include a `Bearer` JWT in the `Authorization` header. Authentication is optional - when no issuer URL or token is configured, requests are sent without authentication.

The Keycloak `dcm` realm provides a public client `dcm-cli` (no client secret) with device authorization enabled. The client ID is hardcoded and not configurable.

### 13.2 Interactive Login

For human users, `dcm login` performs the device authorization flow:

```bash
dcm login --issuer-url https://keycloak.example.com/realms/dcm --control-plane-url https://dcm.example.com
```

This discovers the OIDC provider endpoints, initiates a device flow, opens a browser for the user to authenticate, and stores the resulting tokens. On success, `issuer-url` is persisted to `~/.dcm/config.yaml`, and `control-plane-url` is persisted when explicitly set via flag or `DCM_CONTROL_PLANE_URL`.

### 13.3 CI/Scripting (Static Token)

For CI pipelines and automation, use `DCM_TOKEN` to inject a pre-obtained Bearer token directly:

```bash
export DCM_TOKEN="<bearer-token>"
export DCM_CONTROL_PLANE_URL="https://dcm.example.com"
dcm policy list
```

The static token path bypasses the device flow entirely - no refresh logic, no keychain, no config file interaction. Tokens can be obtained externally via the `dcm-proxy` confidential Keycloak client using a `client_credentials` grant.

### 13.4 Token Storage

Tokens are stored using a two-tier strategy:

1. **OS keychain** (primary) - macOS Keychain, Linux Secret Service (GNOME Keyring/KDE Wallet), Windows Credential Manager. Service name: `dcm-cli`, key: normalized issuer URL.
2. **File** (fallback) - `~/.dcm/tokens.json` with `0600` permissions. Activated automatically when the keychain is unavailable (containers, CI, headless SSH).

The file backend uses atomic writes (write to `.tmp` then rename) for crash safety. The keyring backend delegates to the OS keychain API (`keyring.Set`).

### 13.5 Token Lifecycle

The `AuthTransport` handles token injection lazily during HTTP requests:

1. **Fast path (no network)**: decode the JWT `exp` claim without verification. If the access token is valid (with 30s clock skew buffer), inject it directly.
2. **Refresh path**: if expired, use the stored refresh token to obtain a new access token from the token endpoint. Save the refreshed tokens.
3. **Failure path**: if refresh fails, return an actionable error: `"authentication expired, run 'dcm login' to re-authenticate"`.

This design avoids network calls on every CLI invocation and keeps auth decoupled from the command tree (no `PersistentPreRunE` annotations needed).

### 13.6 Security

- Tokens are never written to the config file (`Token` has `yaml:"-"`)
- The CLI warns on stderr when sending a Bearer token over unencrypted HTTP
- `TokenData.String()` returns `[REDACTED]` to prevent accidental logging
- Token file permissions are set to `0600` (owner read/write only)
- Atomic file writes prevent partial-write corruption
