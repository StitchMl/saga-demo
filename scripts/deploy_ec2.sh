#!/bin/bash

# Script to deploy the project to an EC2 instance.
# Uses 'git clone' for a clean deployment.

# Fail fast
set -e

# --- Configuration ---
# These values can be passed as arguments.
# Example: ./deploy_ec2.sh ubuntu@ec2-xx-xx-xx-xx.compute-1.amazonaws.com /path/to/key.pem

EC2_TARGET=${1:-"ubuntu@ec2-xx-xx-xx-xx.compute-1.amazonaws.com"} # EC2 user and host
SSH_KEY_PATH=${2:-"~/path/to/your-key.pem"} # Path to the SSH key
REPO_URL="https://github.com/StitchMl/saga-demo.git" # Git repository URL
PROJECT_NAME="saga-demo" # Project folder name
PROJECT_DIR="/home/ubuntu/${PROJECT_NAME}" # Destination path on EC2

# --- Helper Functions ---
log_info() {
    echo "INFO: $1"
}

log_error() {
    echo "ERROR: $1" >&2
    exit 1
}

# --- Deployment Steps ---

log_info "Verifying SSH connectivity to EC2 instance: ${EC2_TARGET}..."
if ! ssh -i "$SSH_KEY_PATH" -o ConnectTimeout=10 "${EC2_TARGET}" "exit"; then
    log_error "Could not connect to EC2 instance. Check IP/hostname, user, SSH key, and permissions."
fi
log_info "SSH connection successful."

log_info "Installing Docker, Docker Compose, and Git on EC2 instance (if not already present)..."
ssh -i "$SSH_KEY_PATH" "${EC2_TARGET}" << 'EOF'
    set -e # Fail fast in the remote block as well
    if ! command -v docker &> /dev/null; then
        echo "Installing Docker..."
        sudo apt-get update -y
        sudo apt-get install -y docker.io
        sudo systemctl start docker
        sudo systemctl enable docker
        sudo usermod -aG docker $USER
        echo "Docker installed. A new login might be required to use Docker without 'sudo'."
    else
        echo "Docker is already installed."
    fi

    # Check for docker compose v2 plugin first, then fallback to v1
    if ! docker compose version &> /dev/null; then
        if ! command -v docker-compose &> /dev/null; then
            echo "Installing Docker Compose..."
            sudo apt-get install -y docker-compose
        else
            echo "Docker Compose (v1) is already installed."
        fi
    else
        echo "Docker Compose (v2 plugin) is already installed."
    fi

    if ! command -v git &> /dev/null; then
        echo "Installing Git..."
        sudo apt-get install -y git
    else
        echo "Git is already installed."
    fi
EOF
log_info "Prerequisites check completed."

log_info "Cleaning up existing project on EC2 (if present)..."
ssh -i "$SSH_KEY_PATH" "${EC2_TARGET}" "rm -rf ${PROJECT_DIR}"
log_info "Cleanup complete."

log_info "Cloning Git repository onto EC2 instance..."
ssh -i "$SSH_KEY_PATH" "${EC2_TARGET}" "git clone ${REPO_URL} ${PROJECT_DIR}"
log_info "Repository cloning complete."

log_info "Starting Docker Compose on EC2 instance..."
# Use sudo to avoid permissions issues with the docker socket after a fresh install.
# Detect and use 'docker compose' (v2) if available, otherwise fallback to 'docker-compose' (v1).
ssh -i "$SSH_KEY_PATH" "${EC2_TARGET}" << 'EOF'
    set -e
    cd /home/ubuntu/saga-demo

    COMPOSE_CMD="sudo docker-compose"
    if docker compose version &> /dev/null; then
        COMPOSE_CMD="sudo docker compose"
    fi

    echo "Using '$COMPOSE_CMD' to start the application..."
    $COMPOSE_CMD up -d --build
EOF
log_info "Docker Compose started successfully on the EC2 instance."

EC2_HOST=$(echo $EC2_TARGET | cut -d'@' -f2)
log_info "Deployment completed successfully!"
echo "You can check the status of the services with: ssh -i $SSH_KEY_PATH ${EC2_TARGET} 'cd ${PROJECT_DIR} && sudo docker-compose ps'"
echo "Access the application at: http://${EC2_HOST}:8090"
echo "Access the RabbitMQ UI at: http://${EC2_HOST}:15672 (user: guest, password: guest)"
