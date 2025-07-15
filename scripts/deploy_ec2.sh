#!/bin/bash

# Example script for deployment on an EC2 instance

# Configure these variables
EC2_USER="ubuntu" # Or 'ec2-user', depending on the AMI
EC2_HOST="ec2-XX-XX-XX-XX.compute-1.amazonaws.com" # Replace with the IP/Hostname of your EC2 instance
SSH_KEY_PATH="~/path/to/your-key.pem" # Replace with the path to your SSH key
PROJECT_DIR="/home/${EC2_USER}/SDCC_ProgettoB2" # Path where the project will be cloned on EC2

# --- Auxiliary Functions ---
log_info() {
    echo "INFO: $1"
}

log_error() {
    echo "ERROR: $1" >&2
    exit 1
}

# --- Deployment Steps ---

log_info "Verification of SSH connectivity with EC2 instance..."
if ! ssh -i "$SSH_KEY_PATH" -o ConnectTimeout=5 "${EC2_USER}@${EC2_HOST}" "exit"; then
    log_error "Unable to connect to EC2 instance. Check IP, SSH key and permissions."
fi
log_info "SSH connection successful."

log_info "Docker and Docker Compose installation on the EC2 instance (if not already present)..."
ssh -i "$SSH_KEY_PATH" "${EC2_USER}@${EC2_HOST}" << EOF
    sudo apt-get update -y
    sudo apt-get install -y docker.io docker-compose
    sudo systemctl start docker
    sudo systemctl enable docker
    sudo usermod -aG docker ${EC2_USER}
EOF
if [ $? -ne 0 ]; then log_error "Error during installation of Docker/Docker Compose."; fi
log_info "Docker and Docker Compose installed/verified on EC2."

log_info "Cleaning the existing project on EC2 (if any)..."
ssh -i "$SSH_KEY_PATH" "${EC2_USER}@${EC2_HOST}" "rm -rf ${PROJECT_DIR}"
log_info "Cleaning completed."

log_info "Copying project files to EC2 instance..."
# We assume that the project is in the current directory from where you run the script
scp -i "$SSH_KEY_PATH" -r . "${EC2_USER}@${EC2_HOST}:${PROJECT_DIR}"
if [ $? -ne 0 ]; then log_error "Error while copying project files."; fi
log_info "Copying of files completed."

log_info "Running Docker Compose on EC2 instance..."
ssh -i "$SSH_KEY_PATH" "${EC2_USER}@${EC2_HOST}" "cd ${PROJECT_DIR} && docker-compose up -d --build"
if [ $? -ne 0 ]; then log_error "Error while running Docker Compose."; fi
log_info "Docker Compose successfully started on EC2 instance."

log_info "Distribuzione completata con successo!"
echo "You can check the status of services with: ssh -i $SSH_KEY_PATH ${EC2_USER}@${EC2_HOST} 'docker-compose -f ${PROJECT_DIR}/docker-compose.yml ps'"
echo "Access the RabbitMQ Management UI at: http://${EC2_HOST}:15672 (username: guest, password: guest)"
echo "Access the Order Service (Orchestrator) at: http://${EC2_HOST}:8080/create_order"
echo "Access the Order Service (Choreographer) at: http://${EC2_HOST}:8082/create_order"