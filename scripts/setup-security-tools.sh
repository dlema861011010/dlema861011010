#!/bin/bash

# Security Tools Setup Script for FuseFlash Development
# This script installs and configures local security scanning tools

set -e

echo "🔒 FuseFlash Security Tools Setup"
echo "=================================="
echo ""

# Detect OS
OS="$(uname -s)"
echo "Detected OS: $OS"
echo ""

# Function to check if command exists
command_exists() {
    command -v "$1" >/dev/null 2>&1
}

# Install git-secrets
install_git_secrets() {
    echo "📦 Installing git-secrets..."

    if command_exists git-secrets; then
        echo "✅ git-secrets already installed"
        return
    fi

    case $OS in
        Darwin*)
            if command_exists brew; then
                brew install git-secrets
            else
                echo "Please install Homebrew first: https://brew.sh"
                exit 1
            fi
            ;;
        Linux*)
            git clone https://github.com/awslabs/git-secrets /tmp/git-secrets
            cd /tmp/git-secrets
            sudo make install
            cd -
            rm -rf /tmp/git-secrets
            ;;
        *)
            echo "Unsupported OS for automatic git-secrets installation"
            echo "Please install manually: https://github.com/awslabs/git-secrets"
            return
            ;;
    esac

    echo "✅ git-secrets installed"
}

# Install gitleaks
install_gitleaks() {
    echo "📦 Installing gitleaks..."

    if command_exists gitleaks; then
        echo "✅ gitleaks already installed"
        return
    fi

    case $OS in
        Darwin*)
            if command_exists brew; then
                brew install gitleaks
            fi
            ;;
        Linux*)
            GITLEAKS_VERSION="8.18.0"
            wget "https://github.com/gitleaks/gitleaks/releases/download/v${GITLEAKS_VERSION}/gitleaks_${GITLEAKS_VERSION}_linux_x64.tar.gz"
            tar -xzf "gitleaks_${GITLEAKS_VERSION}_linux_x64.tar.gz"
            sudo mv gitleaks /usr/local/bin/
            rm "gitleaks_${GITLEAKS_VERSION}_linux_x64.tar.gz"
            ;;
    esac

    echo "✅ gitleaks installed"
}

# Install Go security tools
install_go_tools() {
    echo "📦 Installing Go security tools..."

    if ! command_exists go; then
        echo "⚠️  Go not found - skipping Go security tools"
        return
    fi

    # gosec
    if ! command_exists gosec; then
        go install github.com/securego/gosec/v2/cmd/gosec@latest
        echo "✅ gosec installed"
    else
        echo "✅ gosec already installed"
    fi

    # Nancy for dependency scanning
    if ! command_exists nancy; then
        go install github.com/sonatype-nexus-community/nancy@latest
        echo "✅ nancy installed"
    else
        echo "✅ nancy already installed"
    fi
}

# Install Python security tools
install_python_tools() {
    echo "📦 Installing Python security tools..."

    if ! command_exists python3 && ! command_exists python; then
        echo "⚠️  Python not found - skipping Python security tools"
        return
    fi

    PYTHON_CMD=$(command_exists python3 && echo "python3" || echo "python")
    PIP_CMD=$(command_exists pip3 && echo "pip3" || echo "pip")

    # Install bandit
    if ! command_exists bandit; then
        $PIP_CMD install bandit[toml]
        echo "✅ bandit installed"
    else
        echo "✅ bandit already installed"
    fi

    # Install safety
    if ! command_exists safety; then
        $PIP_CMD install safety
        echo "✅ safety installed"
    else
        echo "✅ safety already installed"
    fi
}

# Install Node.js security tools
install_node_tools() {
    echo "📦 Installing Node.js security tools..."

    if ! command_exists npm; then
        echo "⚠️  npm not found - skipping Node.js security tools"
        return
    fi

    # ESLint with security plugin
    npm install -g eslint eslint-plugin-security

    echo "✅ Node.js security tools installed"
}

# Install Solidity security tools
install_solidity_tools() {
    echo "📦 Installing Solidity security tools..."

    if ! command_exists python3 && ! command_exists python; then
        echo "⚠️  Python not found - skipping Solidity security tools"
        return
    fi

    PIP_CMD=$(command_exists pip3 && echo "pip3" || echo "pip")

    # Install Slither
    if ! command_exists slither; then
        $PIP_CMD install slither-analyzer
        echo "✅ slither installed"
    else
        echo "✅ slither already installed"
    fi

    # Install solc-select for managing Solidity versions
    if ! command_exists solc-select; then
        $PIP_CMD install solc-select
        echo "✅ solc-select installed"
    else
        echo "✅ solc-select already installed"
    fi
}

# Configure git-secrets for this repository
configure_git_secrets() {
    echo "🔧 Configuring git-secrets for this repository..."

    if ! command_exists git-secrets; then
        echo "⚠️  git-secrets not installed, skipping configuration"
        return
    fi

    # Install hooks
    git secrets --install -f 2>/dev/null || true

    # Register AWS patterns
    git secrets --register-aws 2>/dev/null || true

    # Add custom patterns from .git-secrets-patterns
    if [ -f ".git-secrets-patterns" ]; then
        while IFS= read -r pattern; do
            # Skip empty lines and comments
            [[ -z "$pattern" || "$pattern" =~ ^# ]] && continue
            git secrets --add "$pattern" 2>/dev/null || true
        done < .git-secrets-patterns
    fi

    echo "✅ git-secrets configured"
}

# Set up pre-commit hook
setup_precommit_hook() {
    echo "🔧 Setting up pre-commit security hook..."

    HOOK_FILE=".git/hooks/pre-commit"

    cat > "$HOOK_FILE" << 'HOOK_EOF'
#!/bin/bash
# Pre-commit security checks for FuseFlash

echo "🔒 Running security checks..."

EXIT_CODE=0

# Run gitleaks if available
if command -v gitleaks >/dev/null 2>&1; then
    echo "  Scanning for secrets with gitleaks..."
    if ! gitleaks protect --staged --verbose --no-banner; then
        echo "❌ Gitleaks found potential secrets!"
        EXIT_CODE=1
    fi
fi

# Run git-secrets if available
if command -v git-secrets >/dev/null 2>&1; then
    echo "  Scanning for secrets with git-secrets..."
    if ! git secrets --pre_commit_hook -- "$@"; then
        echo "❌ git-secrets found potential secrets!"
        EXIT_CODE=1
    fi
fi

# Check for .env files
if git diff --cached --name-only | grep -E "\.env$|\.env\..*"; then
    echo "❌ Attempting to commit .env file!"
    echo "   .env files should never be committed"
    EXIT_CODE=1
fi

# Go security checks
if command -v gosec >/dev/null 2>&1; then
    if git diff --cached --name-only | grep -E "\.go$" >/dev/null; then
        echo "  Running Go security checks..."
        if ! gosec -quiet ./... 2>/dev/null; then
            echo "⚠️  gosec found potential issues (review above)"
        fi
    fi
fi

# Python security checks
if command -v bandit >/dev/null 2>&1; then
    if git diff --cached --name-only | grep -E "\.py$" >/dev/null; then
        echo "  Running Python security checks..."
        if ! bandit -ll -r . 2>/dev/null; then
            echo "⚠️  bandit found potential issues (review above)"
        fi
    fi
fi

if [ $EXIT_CODE -ne 0 ]; then
    echo ""
    echo "❌ Security checks failed! Commit blocked."
    echo "   Fix the issues above and try again."
    exit 1
fi

echo "✅ Security checks passed"
exit 0
HOOK_EOF

    chmod +x "$HOOK_FILE"
    echo "✅ Pre-commit hook installed"
}

# Create .env file from template if it doesn't exist
setup_env_file() {
    echo "🔧 Setting up environment file..."

    if [ -f ".env" ]; then
        echo "✅ .env file already exists"
    elif [ -f ".env.example" ]; then
        cp .env.example .env
        echo "✅ Created .env from .env.example"
        echo "⚠️  Please update .env with your actual values"
    else
        echo "⚠️  No .env.example found"
    fi

    # Ensure .env is in .gitignore
    if [ -f ".gitignore" ]; then
        if ! grep -q "^\.env$" .gitignore; then
            echo ".env" >> .gitignore
            echo "✅ Added .env to .gitignore"
        fi
    else
        echo ".env" > .gitignore
        echo "✅ Created .gitignore with .env"
    fi
}

# Main installation flow
main() {
    # Check if we're in a git repository
    if [ ! -d ".git" ]; then
        echo "❌ Not a git repository. Please run this script from the repository root."
        exit 1
    fi

    echo "Installing security tools..."
    echo ""

    install_git_secrets
    install_gitleaks
    install_go_tools
    install_python_tools
    install_node_tools
    install_solidity_tools

    echo ""
    echo "Configuring repository..."
    echo ""

    configure_git_secrets
    setup_precommit_hook
    setup_env_file

    echo ""
    echo "=================================="
    echo "✅ Security tools setup complete!"
    echo "=================================="
    echo ""
    echo "Next steps:"
    echo "1. Update your .env file with actual values (never commit this!)"
    echo "2. Run security scans before committing:"
    echo "   - gitleaks protect --staged (check for secrets)"
    echo "   - gosec ./... (Go security)"
    echo "   - bandit -r . (Python security)"
    echo "   - npm audit (Node.js dependencies)"
    echo "3. Pre-commit hook will run automatically on 'git commit'"
    echo ""
    echo "For CI/CD, the GitHub Actions workflow will run comprehensive scans."
    echo ""
}

# Run main function
main "$@"
