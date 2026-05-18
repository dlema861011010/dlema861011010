# Security Policy

## GitHub Advanced Security Configuration

This repository is configured with GitHub Advanced Security (GHAS) for comprehensive automated security scanning on GitHub Enterprise Server 3.20+.

## Supported Security Features

### 🔍 Code Scanning (CodeQL)
We use GitHub's CodeQL engine to automatically scan for vulnerabilities in:
- **Go** (FuseFlash P2P node, validation engine)
- **JavaScript/TypeScript** (Ember.js smart wallet, node scripts)
- **Python** (FastAPI bridge, deployment scripts)

CodeQL runs on every push, pull request, and daily at 2 AM UTC.

### 🔐 Secret Scanning & Push Protection
**Active Protection Against:**
- Stripe API keys (sk_test_, sk_live_, pk_*, whsec_*)
- Database credentials and connection strings
- USDC/Ethereum private keys
- AWS access keys
- Generic API tokens and secrets
- SSH private keys

**Push Protection is ENABLED** - commits containing secrets will be blocked before they reach the repository.

### 🛡️ Dependency Scanning
Automated vulnerability scanning for:
- **Go modules** via Nancy (Sonatype)
- **Python packages** via Safety
- **npm packages** via npm audit

### 🔒 Smart Contract Security (Solidity)
**Slither static analysis** runs on all Solidity contracts to detect:
- Reentrancy vulnerabilities
- Uninitialized storage variables
- Access control issues
- Integer overflow/underflow
- Unsafe delegatecall usage
- Timestamp dependence

### 📦 Container Security
**Trivy scanning** for:
- Vulnerable base images
- Outdated system packages
- Known CVEs in dependencies

## Reporting a Vulnerability

### For External Researchers

If you discover a security vulnerability, please **DO NOT** open a public issue.

Instead:

1. **Email**: security@[yourdomain].com (or appropriate contact)
2. **Include**:
   - Description of the vulnerability
   - Steps to reproduce
   - Potential impact
   - Suggested fix (if available)

We will acknowledge receipt within 48 hours and provide a detailed response within 7 days.

### Responsible Disclosure Guidelines

- Allow us reasonable time to fix the vulnerability before public disclosure
- Do not exploit the vulnerability beyond what's necessary to demonstrate it
- Do not access, modify, or delete data belonging to others
- Do not perform DoS attacks

### Bug Bounty

We appreciate security research and will:
- Credit researchers who responsibly disclose vulnerabilities (with permission)
- Provide timely updates on remediation progress
- Consider bug bounty rewards for critical findings (if program is active)

## Security Workflow for Developers

### Before Committing

1. **Run security checks locally:**
   ```bash
   # Go
   cd fuseflash && go vet ./...

   # Python
   cd bridge && bandit -r .

   # JavaScript
   cd smart-wallet && npm audit
   ```

2. **Never commit:**
   - `.env` files with real credentials
   - `credentials.json` or `secrets.json`
   - Private keys or API tokens
   - Database connection strings with passwords

3. **Use environment variables:**
   ```bash
   # Good ✅
   DATABASE_URL="${DATABASE_URL}"

   # Bad ❌
   DATABASE_URL="postgres://user:password@prod.example.com/db"
   ```

### During Pull Request

All PRs automatically trigger:
- CodeQL security scanning
- Secret detection
- Dependency vulnerability checks
- Lint security rules

**PRs will be blocked if:**
- Secrets are detected
- Critical vulnerabilities are introduced
- Security tests fail

### Before Release

Before creating a release:

1. ✅ All security scans must pass
2. ✅ No HIGH or CRITICAL vulnerabilities
3. ✅ Dependencies updated to patch known CVEs
4. ✅ Code review by at least one other developer
5. ✅ Slither analysis shows no critical contract issues

### Creating Immutable Releases (GHES 3.20+)

Once security validated:

```bash
# Tag the secure build
git tag -a v1.0.0-audited -m "Security audited release - $(date)"
git push origin v1.0.0-audited
```

Then create a GitHub release:
- Attach compiled binaries and deployment artifacts
- Include security audit summary in release notes
- Release becomes **immutable** - cryptographic audit trail

## Security Best Practices

### For Smart Contracts (Solidity)

1. **Always use latest stable Solidity compiler**
2. **Implement checks-effects-interactions pattern** to prevent reentrancy
3. **Use SafeMath or Solidity 0.8+** for overflow protection
4. **Limit external calls** and validate all inputs
5. **Follow OpenZeppelin patterns** for standard functionality
6. **Run Slither** before every deployment
7. **Consider formal verification** for critical contracts

### For FuseFlash Node (Go)

1. **Validate all P2P message inputs**
2. **Use crypto/rand** for randomness, never math/rand for security
3. **Implement rate limiting** on all endpoints
4. **Use TLS for all network communication**
5. **Run `go vet` and security linters** before commit

### For Bridge API (Python)

1. **Validate and sanitize all inputs**
2. **Use parameterized queries** to prevent SQL injection
3. **Implement proper CORS policies**
4. **Rate limit all endpoints**
5. **Use secrets management** for API keys (never hardcode)
6. **Keep dependencies updated**

### For Smart Wallet (Ember.js)

1. **Sanitize user inputs** to prevent XSS
2. **Use Content Security Policy (CSP)**
3. **Implement CSRF protection**
4. **Audit npm dependencies** regularly
5. **Use Subresource Integrity (SRI)** for CDN resources

## Compliance & Audit Trail

### Automated Audit Logging

All security scans are logged with:
- Timestamp
- Commit SHA
- Scan results
- Artifacts retained for 90 days

### Access Controls

- **Secret scanning**: Enterprise administrators only
- **Security alerts**: Repository maintainers
- **CodeQL results**: All developers with read access

### Regulatory Compliance

For environments requiring compliance (SOC 2, ISO 27001, PCI DSS):

1. **Enable audit logging** in GHES admin console
2. **Export security scan results** monthly
3. **Document remediation** of all findings
4. **Maintain immutable releases** as evidence

## Security Tools Configuration

### Local Development Setup

Install these tools for local security scanning:

```bash
# Go security
go install github.com/securego/gosec/v2/cmd/gosec@latest

# Python security
pip install bandit safety

# JavaScript security
npm install -g eslint eslint-plugin-security

# Solidity security
pip install slither-analyzer

# Secret detection
brew install gitleaks  # or apt-get install gitleaks
```

### Pre-commit Hooks

Add to `.git/hooks/pre-commit`:

```bash
#!/bin/bash
# Run security checks before commit

echo "Running security checks..."

# Check for secrets
if command -v gitleaks &> /dev/null; then
    gitleaks protect --staged --verbose
fi

# Run language-specific checks
if [ -f "go.mod" ]; then
    gosec ./...
fi

if [ -f "requirements.txt" ]; then
    bandit -r . -ll
fi

echo "✅ Security checks passed"
```

## Incident Response

### If a Secret is Committed

1. **IMMEDIATELY rotate the compromised secret**
2. **Notify security team**
3. **Remove secret from git history:**
   ```bash
   # Use BFG Repo-Cleaner or git filter-branch
   # Contact repository admin for assistance
   ```
4. **Force push cleaned history** (coordinate with team)
5. **Monitor for unauthorized access** using the old secret

### If a Vulnerability is Discovered

1. **Create private security advisory** on GitHub
2. **Assign severity level** (Critical/High/Medium/Low)
3. **Develop fix in private fork**
4. **Test fix thoroughly**
5. **Coordinate disclosure timeline**
6. **Publish security advisory** after fix is deployed

## Contact

**Security Team**: security@[yourdomain].com
**Emergency Contact**: +1-XXX-XXX-XXXX
**PGP Key**: [Link to public key]

---

**Last Updated**: 2026-05-18
**Next Review**: 2026-08-18
**Version**: 1.0.0
