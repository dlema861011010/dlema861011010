# GitHub Advanced Security Configuration

This directory contains configuration for GitHub Advanced Security (GHAS) features available on GitHub Enterprise Server 3.20+.

## Features Enabled

### 1. CodeQL Analysis (`security-audit.yml`)
- **Languages**: Go, JavaScript/TypeScript, Python
- **Query Packs**: Security-focused extended query packs
- **Triggers**: Push, PR, daily schedule, manual dispatch
- **Custom Config**: `.github/codeql/codeql-config.yml`

### 2. Solidity Smart Contract Scanning
- **Tool**: Slither (Trail of Bits)
- **Scans**: Custom FuseFlash payment routing contracts
- **Detects**: Reentrancy, uninitialized storage, access control issues
- **Reports**: JSON + human-readable summary

### 3. Secret Scanning & Push Protection
- **Native GHAS**: Enabled at enterprise/repository level
- **Additional Tool**: TruffleHog for verified secrets
- **Protects**:
  - Stripe secret keys
  - Database credentials
  - USDC deployment keys
  - API tokens
  - Private keys

### 4. Dependency Vulnerability Scanning
- **Go**: Nancy (Sonatype)
- **Python**: Safety
- **Node.js**: npm audit
- **Triggers**: Automatically on dependency changes

### 5. Security Linting
- **Python**: Bandit (detects security issues in Python code)
- **JavaScript/TypeScript**: ESLint with security plugin
- **Shell Scripts**: ShellCheck
- **Focus**: Input validation, injection vulnerabilities, unsafe operations

### 6. Container Security
- **Tool**: Trivy (Aqua Security)
- **Scans**: Dockerfile and container images
- **Uploads**: SARIF results to GitHub Security tab

## Setup Instructions

### Prerequisites
1. GitHub Enterprise Server 3.20+ or 3.21 RC
2. GitHub Advanced Security license enabled
3. Actions runners with internet access (for tool downloads)

### Enable Push Protection
1. Go to Enterprise Settings → Code security and analysis
2. Enable "Secret scanning"
3. Enable "Push protection"
4. Configure custom patterns if needed for your specific keys

### Configure the Workflow

1. **Update branch names** in `security-audit.yml` if your main branches differ:
   ```yaml
   branches: [ "main", "develop", "release/**" ]
   ```

2. **Adjust Solidity compiler version** in the workflow if needed:
   ```bash
   solc-select install 0.8.20  # Change to your version
   ```

3. **Customize paths** in `codeql-config.yml` to match your repository structure

### Running the Workflow

**Automatic triggers:**
- Every push to main, develop, or release branches
- Every pull request to main or develop
- Daily at 2:00 AM UTC
- Manual dispatch from Actions tab

**Manual run:**
1. Navigate to Actions tab
2. Select "Security Audit Pipeline"
3. Click "Run workflow"
4. Select branch and confirm

## Viewing Results

### CodeQL Findings
- Navigate to **Security** → **Code scanning alerts**
- Filter by tool: CodeQL
- Review severity and remediation guidance

### Secret Scanning Findings
- Navigate to **Security** → **Secret scanning alerts**
- Review detected secrets
- Revoke and rotate compromised credentials

### Dependency Alerts
- Navigate to **Security** → **Dependabot alerts**
- Review vulnerable dependencies
- Apply suggested updates

### Artifact Reports
All scan results are uploaded as artifacts:
- **Slither reports**: `slither-security-report`
- **Lint reports**: `security-lint-reports`
- **Complete audit**: `complete-security-audit-{SHA}`

Access via: Actions → workflow run → Artifacts section

## Creating Immutable Releases (GHES 3.20+)

After your security scans pass:

1. **Tag the verified build:**
   ```bash
   git tag -a v1.0.0-secure -m "Security audited release"
   git push origin v1.0.0-secure
   ```

2. **Create immutable release:**
   - Go to Releases → Draft a new release
   - Select your tag
   - Add release notes including security audit summary
   - Attach compiled deployment assets
   - Publish release

3. **Lock the release** (GHES 3.20+ feature):
   - Once published, the release and its assets become immutable
   - Provides cryptographic audit trail
   - Cannot be modified or deleted by anyone

## Customization for Your Environment

### For FuseFlash Contracts
The workflow automatically detects Solidity files in:
- `contracts/`
- `smart-wallet/`

Adjust the Slither command in the workflow if your contracts are elsewhere.

### For Node Validation Engine
The validation script security job scans:
- Python scripts (Bandit)
- JavaScript/TypeScript (ESLint security plugin)
- Shell scripts (ShellCheck)

Add custom security rules in `.eslintrc.json` or `bandit.yml` as needed.

## Troubleshooting

**CodeQL fails on custom code:**
- Ensure your code compiles successfully
- Check `codeql-config.yml` paths match your structure
- Review build logs for compilation errors

**Slither not finding contracts:**
- Verify Solidity files exist in expected directories
- Check Solidity version compatibility
- Ensure contracts compile with standard solc

**Secret scanning false positives:**
- Add patterns to `.github/secret_scanning.yml` ignore list
- Use GitHub's custom pattern configuration

## Security Best Practices

1. **Never disable security checks** to merge faster
2. **Rotate all secrets** found by scanners immediately
3. **Review all HIGH/CRITICAL findings** before release
4. **Keep dependencies updated** to patch vulnerabilities
5. **Use immutable releases** for production deployments
6. **Enable branch protection** requiring security checks to pass

## Support

For issues with:
- **GHAS features**: Contact your GHES administrator
- **Workflow configuration**: Review Actions logs and job summaries
- **False positives**: Use issue templates to report and track

---

**Generated for GitHub Enterprise Server 3.20+ with GitHub Advanced Security**
