#!/bin/bash

# Script to deploy the project to an EC2 instance.
# Uses 'git clone' for a clean deployment.

# Fail fast
set -e

# Get the directory where the script is located
SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )
KEY_DIR="${SCRIPT_DIR}/../keys" # Define the keys directory

# --- Configuration ---
# Validate that the required arguments are provided.
if [ -z "$1" ] || [ -z "$2" ]; then
    echo "ERROR: Missing required arguments."
    echo "Usage: $0 <user> <public-ip>"
    echo "Example: $0 ubuntu 18.209.19.184"
    echo "Note: The script automatically finds a single .pem key file in the keys directory (${KEY_DIR})."
    exit 1
fi

EC2_USER=$1
EC2_HOST=$2
EC2_TARGET="${EC2_USER}@${EC2_HOST}" # EC2 user and host (e.g., ubuntu@18.209.19.184)

# --- Helper Functions ---
log_info() {
    echo "INFO: $1"
}

log_error() {
    echo "ERROR: $1" >&2
    exit 1
}

# --- Find SSH Key ---
log_info "Searching for SSH key file (.pem) in ${KEY_DIR}..."
# Find .pem files in the keys directory, handle spaces in names, store in array
PEM_FILES=()
while IFS= read -r -d $'\0'; do
    PEM_FILES+=("$REPLY")
done < <(find "$KEY_DIR" -maxdepth 1 -type f -name "*.pem" -print0)

# Check the number of .pem files found
if [ ${#PEM_FILES[@]} -eq 0 ]; then
    log_error "No .pem key file found. Please place your single EC2 private key file in the key directory: ${KEY_DIR}"
fi

if [ ${#PEM_FILES[@]} -gt 1 ]; then
    # When multiple keys are found, list them for clarity and then report an error.
    echo "Found files:"
    printf " - %s\n" "${PEM_FILES[@]##*/}"
    log_error "Multiple .pem key files found. Please ensure only one .pem key file is present in the key directory: ${KEY_DIR}"
fi

SSH_KEY_PATH="${PEM_FILES[0]}"
log_info "Using SSH key: ${SSH_KEY_PATH}"

# --- Create a secure temporary copy of the key ---
# This is necessary when running from WSL on a Windows filesystem (/mnt/c)
# where 'chmod' on the original file may not work as expected by SSH.
TEMP_KEY_PATH=$(mktemp)
# On exit, ensure the temp key is removed.
trap 'rm -f "$TEMP_KEY_PATH"' EXIT

log_info "Creating a secure temporary copy of the SSH key for this session..."
cat "$SSH_KEY_PATH" > "$TEMP_KEY_PATH"
chmod 600 "$TEMP_KEY_PATH"

# Options to make SSH non-interactive
SSH_OPTIONS="-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null"

REPO_URL="https://github.com/StitchMl/saga-demo.git" # Git repository URL (HTTPS format)
PROJECT_NAME="saga-demo" # Project folder name
PROJECT_DIR="/home/${EC2_USER}/${PROJECT_NAME}" # Destination path on EC2

# --- Deployment Steps ---

log_info "Verifying SSH connectivity to EC2 instance: ${EC2_TARGET}..."
if ! ssh "$SSH_OPTIONS" -i "$TEMP_KEY_PATH" -o ConnectTimeout=10 "${EC2_TARGET}" "exit"; then
    log_error "Could not connect to EC2 instance (Connection Timed Out).
    Please check the following:
    1. The EC2 instance is 'running' and has passed its status checks in the AWS Console.
    2. The Public IP address '${EC2_HOST}' is correct and hasn't changed.
    3. The instance's Security Group has an Inbound Rule allowing TCP traffic on port 22 from your IP.
    4. Your local network or firewall is not blocking outbound traffic on port 22."
fi
log_info "SSH connection successful."

log_info "Installing Docker, Docker Compose, and Git on EC2 instance (if not already present)..."
ssh "$SSH_OPTIONS" -i "$TEMP_KEY_PATH" "${EC2_TARGET}" << 'EOF'
    set -e # Fail fast in the remote block as well

    # --- Detect Package Manager ---
    PKG_MANAGER=""
    INSTALL_CMD=""
    if command -v dnf &> /dev/null; then
        PKG_MANAGER="dnf"
    elif command -v yum &> /dev/null; then
        PKG_MANAGER="yum"
    elif command -v apt-get &> /dev/null; then
        PKG_MANAGER="apt-get"
    else
        echo "ERROR: Could not detect a supported package manager (dnf, yum, apt-get)." >&2
        exit 1
    fi
    echo "Detected package manager: $PKG_MANAGER"

    # --- Installation Logic ---
    if [ "$PKG_MANAGER" = "apt-get" ]; then
        echo "Updating package lists..."
        sudo apt-get update -y
        INSTALL_CMD="sudo apt-get install -y"
    else # dnf or yum
        INSTALL_CMD="sudo $PKG_MANAGER install -y"
    fi

    # --- Install Docker ---
    if ! command -v docker &> /dev/null; then
        echo "Installing Docker..."
        if [ "$PKG_MANAGER" = "apt-get" ]; then
            $INSTALL_CMD docker.io
        else # For dnf/yum
            $INSTALL_CMD docker
        fi
        sudo systemctl start docker
        sudo systemctl enable docker
        sudo usermod -aG docker $USER
        echo "Docker installed. A new login might be required to use Docker without 'sudo'."
    else
        echo "Docker is already installed."
    fi

    # --- Install Docker Compose ---
    # We prefer Docker Compose v2 (the plugin). Check if it's installed.
    if ! docker compose version &> /dev/null; then
        echo "Installing Docker Compose plugin (v2) by downloading from GitHub..."
        # This is the most reliable method across different Linux distributions.
        # Ensure curl is installed
        if ! command -v curl &> /dev/null; then
            echo "Installing curl..."
            $INSTALL_CMD curl
        fi

        # Create the directory for CLI plugins
        DOCKER_CONFIG_DIR="/usr/local/lib/docker/cli-plugins"
        sudo mkdir -p "$DOCKER_CONFIG_DIR"

        # Download the latest stable release of Docker Compose
        COMPOSE_VERSION=$(curl -s https://api.github.com/repos/docker/compose/releases/latest | grep -oP '"tag_name": "\K(v[0-9]+\.[0-9]+\.[0-9]+)')
        if [ -z "$COMPOSE_VERSION" ]; then
            echo "Could not fetch latest Docker Compose version, falling back to a recent stable version."
            COMPOSE_VERSION="v2.27.0" # Fallback version
        fi
        echo "Downloading Docker Compose version ${COMPOSE_VERSION}..."
        sudo curl -SL "https://github.com/docker/compose/releases/download/${COMPOSE_VERSION}/docker-compose-linux-$(uname -m)" -o "${DOCKER_CONFIG_DIR}/docker-compose"

        # Make the binary executable
        sudo chmod +x "${DOCKER_CONFIG_DIR}/docker-compose"

        # Verify installation
        if docker compose version &> /dev/null; then
            echo "Docker Compose plugin installed successfully."
        else
            echo "ERROR: Docker Compose installation failed." >&2
            exit 1
        fi
    else
        echo "Docker Compose (v2 plugin) is already installed."
    fi

    # --- Install Git ---
    if ! command -v git &> /dev/null; then
        echo "Installing Git..."
        $INSTALL_CMD git
    else
        echo "Git is already installed."
    fi
EOF
log_info "Prerequisites check completed."

log_info "Cleaning up existing project on EC2 (if present)..."
ssh "$SSH_OPTIONS" -i "$TEMP_KEY_PATH" "${EC2_TARGET}" "rm -rf ${PROJECT_DIR}"
log_info "Cleanup complete."

log_info "Cloning Git repository onto EC2 instance..."
# Using HTTPS URL. This will work for public repositories.
# Use --depth 1 for a shallow clone and --quiet to suppress progress meter to avoid hangs.
# Use GIT_TERMINAL_PROMPT=0 to ensure it doesn't wait for credentials.
ssh "$SSH_OPTIONS" -i "$TEMP_KEY_PATH" "${EC2_TARGET}" "GIT_TERMINAL_PROMPT=0 git clone --depth 1 --quiet ${REPO_URL} ${PROJECT_DIR}"
log_info "Repository cloning complete."

log_info "Setting up environment and starting the application..."

ssh "$SSH_OPTIONS" -i "$TEMP_KEY_PATH" "${EC2_TARGET}" "PROJECT_DIR=${PROJECT_DIR} bash -s" << 'EOF'
    set -e
    cd "$PROJECT_DIR"
    # Create a minimal .env file for docker-compose. Use the internal Docker network for the frontend to reach the API gateway.
    # This avoids hard-coded localhost references and works in both EC2 and Docker contexts.
    cat > .env <<'EOL'
REACT_APP_API_BASE_URL=http://api-gateway:8000
GATEWAY_PORT=8000
EOL
    # Determine the appropriate docker compose command (v2 or v1) and start the services.
    COMPOSE_CMD="sudo docker compose"
    # Fallback to the legacy binary if the plugin isn't available.
    if ! docker compose version &> /dev/null && command -v docker-compose &> /dev/null; then
        COMPOSE_CMD="sudo docker-compose"
    fi
    echo "Using '$COMPOSE_CMD' to build and start the containers..."
    $COMPOSE_CMD up -d --build
EOF

log_info "Deployment completed successfully!"
echo "Access the frontend application at: http://${EC2_HOST}:8090"
echo "Access the API Gateway at:  http://${EC2_HOST}:8000"