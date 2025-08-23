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

# shellcheck disable=SC2034
REPO_URL="https://github.com/StitchMl/saga-demo.git" # Git repository URL (HTTPS format)
PROJECT_NAME="saga-demo" # Project folder name
PROJECT_DIR="/home/${EC2_USER}/${PROJECT_NAME}" # Destination path on EC2

# Path to the local project root (one level above the script directory). This is used to package the local
# repository when deploying without cloning from GitHub.
LOCAL_PROJECT_ROOT=$(dirname "$SCRIPT_DIR")

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
ssh "${SSH_OPTIONS[@]}" -i "$TEMP_KEY_PATH" "${EC2_TARGET}" "rm -rf ${PROJECT_DIR}"
log_info "Cleanup complete."

log_info "Copying local project onto EC2 instance..."
# Create a temporary archive of the local project directory. The archive is created in /tmp and removed after use.
ARCHIVE_PATH=$(mktemp /tmp/${PROJECT_NAME}_archive.XXXXXX.tar.gz)
tar -czf "$ARCHIVE_PATH" -C "$LOCAL_PROJECT_ROOT" .
# Transfer the archive to the EC2 instance. It will be saved in the home directory of the EC2 user.
scp "${SSH_OPTIONS[@]}" -i "$TEMP_KEY_PATH" "$ARCHIVE_PATH" "${EC2_TARGET}:/home/${EC2_USER}/${PROJECT_NAME}.tar.gz"
# Remove the local temporary archive.
rm -f "$ARCHIVE_PATH"
# On the EC2 host: remove any existing project directory, extract the new archive, and clean up the archive file.
ssh "${SSH_OPTIONS[@]}" -i "$TEMP_KEY_PATH" "${EC2_TARGET}" "rm -rf ${PROJECT_DIR} && mkdir -p /home/${EC2_USER} && tar -xzf ${PROJECT_NAME}.tar.gz -C /home/${EC2_USER} && rm -f ${PROJECT_NAME}.tar.gz"
log_info "Project files copied to EC2."

log_info "Generating Go module files on EC2 instance..."
# The go.mod and go.sum files might not be in git, so we generate them.
# This creates a single module in the backend with replace directives for local packages.
ssh "$SSH_OPTIONS" -i "$TEMP_KEY_PATH" "${EC2_TARGET}" "PROJECT_DIR=${PROJECT_DIR} bash -s" << 'EOF'
    set -e
    # Ensure Go is installed to run go mod commands
    if ! command -v go &> /dev/null; then
        echo "Installing Go..."
        if command -v dnf &> /dev/null; then
            sudo dnf install -y golang
        elif command -v yum &> /dev/null; then
            sudo yum install -y golang
        elif command -v apt-get &> /dev/null; then
            sudo apt-get update -y && sudo apt-get install -y golang
        else
            echo "ERROR: Could not install Go. No supported package manager found." >&2
            exit 1
        fi
    fi

    echo "Initializing Go module in the backend directory..."
    cd "$PROJECT_DIR/backend"
    go mod init github.com/StitchMl/saga-demo

    echo "Adding local replace directives to go.mod..."
    # These directives tell Go to use the local directories for these packages.
    echo "replace github.com/StitchMl/saga-demo/choreographer_saga/shared => ./choreographer_saga/shared" >> go.mod
    echo "replace github.com/StitchMl/saga-demo/common/payment_gateway => ./common/payment_gateway" >> go.mod
    echo "replace github.com/StitchMl/saga-demo/common/types => ./common/types" >> go.mod

    echo "Running 'go mod tidy' to fetch dependencies..."
    go mod tidy

    echo "Vendoring dependencies for Docker build..."
    go mod vendor
EOF
log_info "Go module files generated."

log_info "Building Go binaries on EC2 instance..."
# Pre-build all Go applications on the EC2 host.
# This is more reliable than building inside Docker with complex contexts.
ssh "$SSH_OPTIONS" -i "$TEMP_KEY_PATH" "${EC2_TARGET}" "PROJECT_DIR=${PROJECT_DIR} bash -s" << 'EOF'
    set -e
    cd "$PROJECT_DIR/backend"
    # Find all main.go files and build them.
    find . -type f -name 'main.go' | while read -r mainfile; do
        servicedir=$(dirname "$mainfile")
        servicename=$(basename "$servicedir")
        output_path="$servicedir/$servicename"
        # The package path must be relative to the module defined in go.mod.
        packagepath="github.com/StitchMl/saga-demo/${servicedir#./}"

        echo "Building $packagepath -> $output_path"
        CGO_ENABLED=0 GOOS=linux go build -mod=vendor -o "$output_path" "$packagepath"
    done
EOF
log_info "Go binaries built successfully."


log_info "Patching Dockerfiles to use pre-built binaries..."
# The Dockerfiles are patched to simply copy the pre-built binary,
# skipping the Go build process inside Docker entirely.
ssh "$SSH_OPTIONS" -i "$TEMP_KEY_PATH" "${EC2_TARGET}" "PROJECT_DIR=${PROJECT_DIR} bash -s" << 'EOF'
    set -e
    cd "$PROJECT_DIR"
    # Find all Dockerfiles and patch them
    find . -type f -name 'Dockerfile' | while read -r dockerfile; do
        echo "Patching $dockerfile..."
        # Get the service name from the final part of the directory path
        servicename=$(basename "$(dirname "$dockerfile")")
        # Get the relative path to the service directory from the project root
        servicedir_from_root=$(dirname "${dockerfile#./}")

        # Create a new, simplified Dockerfile content
        new_dockerfile_content=$(cat <<END
# Using a pre-built binary from the EC2 host
FROM alpine:3.19
WORKDIR /root/
# Copy the pre-built binary into the final image.
# The path is relative to the docker-compose.yml file at the project root.
COPY ${servicedir_from_root}/${servicename} .
CMD ["./${servicename}"]
END
)
        # Overwrite the original Dockerfile with the new simplified content
        echo "$new_dockerfile_content" > "$dockerfile"
    done
EOF
log_info "Dockerfiles patched successfully."

log_info "Creating .env file for Docker Compose..."
# This file provides environment variables to the docker-compose services.
# The frontend service should be configured to use this variable to avoid hardcoded URLs.
ssh "$SSH_OPTIONS" -i "$TEMP_KEY_PATH" "${EC2_TARGET}" "PROJECT_DIR=${PROJECT_DIR} EC2_HOST=${EC2_HOST} bash -s" << 'EOF'
    set -e
    # The API Gateway is assumed to be running on port 8090 on the host.
    # The frontend code should use this environment variable to make API calls.
    # This prevents the frontend from redirecting to placeholder URLs like the AWS console.
    echo "GATEWAY_URL=http://${EC2_HOST}:8090" > "${PROJECT_DIR}/.env"
    echo "INFO: Created .env file in ${PROJECT_DIR}"
EOF
log_info ".env file created."

log_info "Starting Docker Compose on EC2 instance..."
# Use sudo to avoid permissions issues with the docker socket after a fresh install.
# Detect and use 'docker compose' (v2) if available, otherwise fallback to 'docker-compose' (v1).
ssh "$SSH_OPTIONS" -i "$TEMP_KEY_PATH" "${EC2_TARGET}" "PROJECT_DIR=${PROJECT_DIR} bash -s" << 'EOF'
    set -e
    cd "$PROJECT_DIR"

    COMPOSE_CMD="sudo docker-compose"
    if docker compose version &> /dev/null; then
        COMPOSE_CMD="sudo docker compose"
    fi

    echo "Using '$COMPOSE_CMD' to start the application..."
    # Build and start all services. The Dockerfiles now handle the vendored dependencies correctly.
    $COMPOSE_CMD up -d --build
EOF
log_info "Docker Compose started successfully on the EC2 instance."

log_info "Waiting for services to initialize and checking their status..."
ssh "$SSH_OPTIONS" -i "$TEMP_KEY_PATH" "${EC2_TARGET}" "PROJECT_DIR=${PROJECT_DIR} bash -s" << 'EOF'
    set -e
    cd "$PROJECT_DIR"

    COMPOSE_CMD="sudo docker-compose"
    if docker compose version &> /dev/null; then
        COMPOSE_CMD="sudo docker compose"
    fi

    echo "--- Waiting for frontend to compile (max 90s)... ---"
    max_wait=90
    interval=5
    elapsed=0
    while true; do
        # Check frontend logs for the success message. The service name is assumed to be 'frontend'.
        if $COMPOSE_CMD logs frontend | grep -q "Compiled successfully!"; then
            echo "Frontend has compiled successfully!"
            break
        fi

        if [ $elapsed -ge $max_wait ]; then
            echo "WARNING: Frontend did not show 'Compiled successfully!' message within ${max_wait} seconds."
            break
        fi

        echo "Waiting for frontend... (elapsed: ${elapsed}s)"
        sleep $interval
        elapsed=$((elapsed + interval))
    done

    echo "--- Container Status (docker-compose ps) ---"
    $COMPOSE_CMD ps
EOF

log_info "Deployment completed successfully!"
# The command for manual ssh connection should use the original key path, not the temporary one.
echo "You can check the status of the services with: ssh -i $SSH_KEY_PATH ${EC2_TARGET} 'cd ${PROJECT_DIR} && sudo docker-compose ps'"
echo "Access the application at: http://${EC2_HOST}:8090"
echo "Access the RabbitMQ UI at: http://${EC2_HOST}:15672 (user: guest, password: guest)"

# --- Final step: Tail logs from the server ---
log_info "Tailing logs from the server. Press Ctrl+C to exit."
ssh "$SSH_OPTIONS" -i "$TEMP_KEY_PATH" "${EC2_TARGET}" "cd ${PROJECT_DIR} && sudo docker-compose logs -f"