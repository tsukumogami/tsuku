# Dockerfile for tsuku testing environment
# Minimal user environment - only basic utilities, no dev tools
FROM ubuntu:22.04

# Avoid interactive prompts
ENV DEBIAN_FRONTEND=noninteractive

# Install only minimal essentials (typical user environment)
RUN apt-get update && \
    apt-get install -y \
        wget \
        curl \
        ca-certificates \
        openssh-server \
        sudo \
        && \
    rm -rf /var/lib/apt/lists/*

# Note: No Go, no git, no build-essential
# This replicates a clean user environment where only tsuku will be installed

# Configure SSH for Vagrant
RUN mkdir /var/run/sshd && \
    # Create vagrant user with password 'vagrant'
    useradd -m -s /bin/bash vagrant && \
    echo 'vagrant:vagrant' | chpasswd && \
    echo 'vagrant ALL=(ALL) NOPASSWD:ALL' >> /etc/sudoers && \
    # Set up SSH key for Vagrant
    mkdir -p /home/vagrant/.ssh && \
    chmod 700 /home/vagrant/.ssh && \
    curl -fsSL https://raw.githubusercontent.com/hashicorp/vagrant/main/keys/vagrant.pub \
        > /home/vagrant/.ssh/authorized_keys && \
    chmod 600 /home/vagrant/.ssh/authorized_keys && \
    chown -R vagrant:vagrant /home/vagrant/.ssh

# Set up environment for vagrant user
# Auto-cd to /vagrant and add it to PATH
RUN echo 'export PATH=/vagrant:$PATH' >> /home/vagrant/.bashrc && \
    echo 'cd /vagrant 2>/dev/null || true' >> /home/vagrant/.bashrc

# Add welcome message
RUN echo '#!/bin/bash\necho "==========================================="\necho "  tsuku Testing Environment (Minimal)"\necho "==========================================="\necho ""\necho "Build tsuku on HOST: go build -o tsuku ./cmd/tsuku"\necho "Then test it here: ./tsuku install <tool>"\necho ""\necho "This environment has NO dev tools installed."\necho "Perfect for testing tsuku in a clean user environment!"' \
        > /home/vagrant/.welcome && \
    chmod +x /home/vagrant/.welcome && \
    chown vagrant:vagrant /home/vagrant/.welcome && \
    echo '~/.welcome' >> /home/vagrant/.bashrc

# Expose SSH port
EXPOSE 22

# Create workspace
WORKDIR /workspace

# Start SSH service
CMD ["/usr/sbin/sshd", "-D"]
