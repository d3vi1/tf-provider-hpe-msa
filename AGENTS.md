You are Codex running with GitHub access and the ability to open issues, create branches, open PRs, run CI locally (when available), and update GitHub Actions workflows.

Goal
Build a production-quality Terraform provider for HPE MSA 2050 arrays (firmware VL270P008) using the array’s embedded HTTPS XML API (command-style endpoints such as login/logout and show/create/delete operations). The provider MUST support:
•⁠  ⁠Creating/deleting LUNs (volumes)
•⁠  ⁠Cloning LUNs (volume clone)
•⁠  ⁠Creating/deleting snapshots of LUNs
•⁠  ⁠Reading and exporting each LUN’s WWID / durable identifier (as exposed by MSA)
•⁠  ⁠Creating/deleting Hosts
•⁠  ⁠Attaching/detaching initiators (FC WWPN and/or iSCSI IQN) to Hosts
•⁠  ⁠Mapping/unmapping volumes to hosts (and optionally host groups if supported) with deterministic behavior

Non-goals / strict constraints
•⁠  ⁠Do NOT use APIs unrelated to LUN/Host manipulation (no “infrastructure” APIs, no unrelated HPE platforms, no generic provisioning layers).
•⁠  ⁠Do NOT use Swordfish/SMI-S/REST endpoints unless you prove they exist AND are required for MSA 2050 XML API volume/host operations. Default to the MSA embedded XML API.
•⁠  ⁠Do NOT implement unrelated features (alerts, fans, PSU, etc.) beyond what you need for volume/host lifecycle and mapping.

Repository bootstrap
1) Create a new GitHub repository named: tf-provider-hpe-msa
2) Use Go and the modern Terraform provider stack:
   - terraform-plugin-framework (preferred)
   - acceptance tests structured per Terraform conventions
3) Initialize a clean module layout, license, README, CONTRIBUTING, CODEOWNERS.
4) Add a .gitignore that explicitly excludes any secrets/config used to reach a real array (examples below). Those secrets WILL be provided by me later, locally or as GitHub Actions secrets.

Project management rules
•⁠  ⁠Create GitHub Milestones and Issues yourself.
•⁠  ⁠Every Issue must be implemented as a separate PR, EXCEPT the bootstrap issue (bootstrap can land directly on main or via an initial PR, your choice).
•⁠  ⁠Each PR must:
  - Pass CI in green
  - Include tests where appropriate
  - Include documentation updates (README + examples) when adding user-facing resources
•⁠  ⁠Do not start the next issue until the previous PR is merged (squash+merge) and the branch is deleted.
•⁠  ⁠If you detect a point where you need user verification (e.g., “I need the real array credentials”, “I need to confirm the exact XML command name for clone/snapshot”, “This operation could destroy data”), STOP and ask for user input. Do not proceed guessing.

CI/CD requirements (must be done in bootstrap)
Set up GitHub Actions with:
•⁠  ⁠go test ./...
•⁠  ⁠golangci-lint (or equivalent) with a committed config
•⁠  ⁠formatting checks (gofmt)
•⁠  ⁠caching for Go modules/build cache
•⁠  ⁠artifacts upload (test logs, coverage) when useful
•⁠  ⁠a “smoke test” job that runs without real array credentials (unit/integration-with-mocks)
•⁠  ⁠optional: a gated acceptance test workflow that only runs when secrets are present (and never on PRs from forks)

Secrets handling / local test harness
•⁠  ⁠Create a local-only config file and/or env-based config, e.g.:
  - .env (gitignored)
  - msa-test-config.yaml (gitignored)
•⁠  ⁠Define these environment variables (names can vary, but be consistent and documented):
  - MSA_ENDPOINT (https://ip-or-fqdn)
  - MSA_USERNAME
  - MSA_PASSWORD
  - MSA_INSECURE_TLS (true/false)
  - MSA_POOL or MSA_VDISK name for volume placement
  - Optional: MSA_TEST_HOST_NAME, MSA_TEST_INITIATOR_WWPN or MSA_TEST_INITIATOR_IQN
•⁠  ⁠Add a Makefile (or task runner) targets:
  - make lint
  - make test
  - make testacc (acceptance tests; requires secrets)
•⁠  ⁠Implement a mock server (or golden XML fixtures) so unit tests do not need a real array.

Development plan (you create the detailed steps)
Create milestones that roughly follow:
Milestone 0: Bootstrap + CI + skeleton provider + docs
Milestone 1: Auth/session + generic XML command client + data sources
Milestone 2: Volumes (LUN) CRUD + read WWID/durable-id
Milestone 3: Snapshots CRUD
Milestone 4: Clones (volume clone from volume and/or snapshot)
Milestone 5: Hosts + initiators CRUD
Milestone 6: Volume mapping/unmapping to hosts (+ optional host groups)
Milestone 7: Acceptance tests against real array + hardening + docs/examples

Implementation requirements
•⁠  ⁠Build a robust XML API client:
  - session management (login/logout)
  - retry/backoff for transient failures
  - timeouts
  - TLS handling (insecure optional; warn)
  - structured parsing of XML to typed structs
•⁠  ⁠Terraform resource design must be idempotent:
  - Prefer stable IDs from the array (volume ID, host ID, mapping ID) rather than names.
  - If only names are available, implement careful lookup and drift behavior.
•⁠  ⁠Provide clear examples in README:
  - provider configuration
  - creating a volume + snapshot + clone
  - creating a host + initiator + mapping
•⁠  ⁠Provide import instructions for each resource.
•⁠  ⁠Ensure destructive operations are explicit and safe:
  - require confirmation fields if needed (or ForceNew semantics)
  - never delete snapshots/volumes implicitly unless Terraform destroy is invoked

Code review loop
For each PR:
1) Run CI, fix failures.
2) Perform a Codex “self review” and apply improvements.
3) Perform a GitHub Copilot-style review simulation (generate a review checklist and address it).
4) Only then merge (squash+merge), delete the branch, and proceed to the next issue.

Stop conditions
•⁠  ⁠If an operation could delete real data unintentionally, stop and ask for user confirmation.
•⁠  ⁠If you cannot discover/confirm the exact XML commands/parameters needed for clone/snapshot/mapping, stop and ask for user input (I can provide command outputs or docs).

Deliverable
At the end, the repository should have:
•⁠  ⁠Working provider build
•⁠  ⁠CI green
•⁠  ⁠Unit tests with mocks
•⁠  ⁠Acceptance tests runnable with provided secrets
•⁠  ⁠Implemented resources:
  - hpe_msa_volume
  - hpe_msa_snapshot
  - hpe_msa_clone (or hpe_msa_volume_clone)
  - hpe_msa_host
  - hpe_msa_host_initiator (or equivalent)
  - hpe_msa_volume_mapping
•⁠  ⁠Data sources for lookup as needed
•⁠  ⁠README + examples directory

Proceed now by creating the repo, then opening the bootstrap issue/milestone, and executing Milestone 0.
