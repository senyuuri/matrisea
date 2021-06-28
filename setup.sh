#!/bin/bash -e
#
# Matrisea - Android Dynamic Analysis Platform
# The script installs dependencies, downloads android-cuttlefish, and builds a default cuttlefish image
#
# (c) Sea Security Team 2021


warn() {
  echo "Warning: $*" >&2
}

exit_w_err() {
  echo "Error: $*" >&2
  echo "Installation aborted"
  exit 1
}

echo "================================================="
echo "Matrisea - Android Dynamic Analysis Platform"
echo "(c) Sea Security Team 2021"
echo "================================================="

echo "[Dependency] Checking OS version..."
if ! which lsb_release &>/dev/null || ! lsb_release -d |grep -q "Ubuntu"; then
  exit_w_err "Matrisea only supports Ubuntu"
fi

echo "[Dependency] Checking CPU VT support..."
if grep -c -w 'vmx\|svm' /proc/cpuinfo | grep -q "0"; then
  exit_w_err "CPU virtualization not enabled. If you're running in a VM, make sure it supports nested-virtualisation"
fi

echo "[Dependency] Checking docker..."
if ! which docker &>/dev/null || ! docker ps |grep -q "CONTAINER"; then
  exit_w_err "Docker is not installed or is not accessible from the current user. Forgot to add the user to the docker group?"
fi

echo "[Install] Install system-level tools and dependencies..."
sudo apt-get install -y -q android-tools-adb android-tools-fastboot

echo "[Install] Downloading android-cuttlefish and adeb..."
mkdir -p deps; cd deps; 
WORKDIR=$(pwd)
if [[ ! -d "adeb" ]]; then
  git clone https://github.com/joelagnel/adeb.git
fi

if [[ ! -d "android-cuttlefish" ]]; then
  git clone https://github.com/google/android-cuttlefish
fi

echo "[Install] Building and installing cuttlefish debian package..."
cd android-cuttlefish 
debuild -i -us -uc -b > /dev/null
sudo dpkg -i ../cuttlefish-common_*_amd64.deb || sudo apt-get install -f -q

echo "[Install] Building cuttlefish VM image..."
sudo apt install -y git
sudo modprobe vhost_vsock vhost_net
./build.sh
cd "${WORKDIR}"; 

echo ""
echo "REBOOT REQUIRED: Seafarm installed successfully. Reboot to install additional kernel modules and apply udev rules."
echo ""
echo "After reboot, run docker-compose up and visit the url below to access the web panel:"
echo ""
echo "      http://127.0.0.1:10080/"
echo ""