#!/bin/bash -e
#
# Matrisea - Android Dynamic Analysis Platform
# The script installs dependencies, downloads android-cuttlefish, and builds a default cuttlefish image
#
# @senyuuri


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
echo "@senyuuri"
echo "================================================="

if ! [ $(id -u) = 0 ]; then
   echo "The script need to be run as root." >&2
   exit 1
fi

if [ $SUDO_USER ]; then
    real_user=$SUDO_USER
else
    real_user=$(whoami)
fi

echo "[Dependency] Checking OS version..."
if ! which lsb_release &>/dev/null || ! lsb_release -d |grep -q "Ubuntu"; then
  exit_w_err "Matrisea only supports Ubuntu for now"
fi

echo "[Dependency] Checking CPU VT support..."
if grep -c -w 'vmx\|svm' /proc/cpuinfo | grep -q "0"; then
  exit_w_err "CPU virtualization not enabled. If you're running in a VM, make sure it supports nested-virtualisation"
fi

echo "[Dependency] Checking docker..."
if ! which docker &>/dev/null || ! docker ps |grep -q "CONTAINER"; then
  exit_w_err "Docker is not installed or is not accessible from the current user. Forgot to add the user to the docker group?"
fi

echo "[Dependency] Load vsock kernel modules..."
if systemctl status open-vm-tools.service | grep -q "Active: active"; then 
  read -p "===| vmware-tool is using vsock. (Recommended) Disable open-vm-tools.service and unload conflicting kernel modules? (y/n)"  -n 1 -r
  echo 
  if [[ $REPLY =~ ^[Yy]$ ]]
  then
      systemctl disable open-vm-tools.service
      echo "===| open-vm-tools.service service diabled"
      exit_w_err "Reboot and run ./setup.sh again"
  fi
fi
modprobe vhost_vsock vhost_net

echo "[Install] Install system-level tools and dependencies..."
apt install -y -q git android-tools-adb android-tools-fastboot build-essential devscripts debhelper-compat golang config-package-dev init-system-helpers=1.5\*

echo "[Install] Downloading android-cuttlefish and adeb..."
sudo -u $real_user mkdir -p deps; cd deps; 
WORKDIR=$(pwd)
if [[ ! -d "adeb" ]]; then
  sudo -u $real_user git clone https://github.com/joelagnel/adeb.git
fi

if [[ ! -d "android-cuttlefish" ]]; then
  sudo -u $real_user git clone https://github.com/google/android-cuttlefish
fi

echo "[Install] Building and installing cuttlefish debian package..."
cd android-cuttlefish 
debuild -i -us -uc -b
dpkg -i ../cuttlefish-common_*_*64.deb || apt install -f
usermod -aG kvm,cvdnetwork,render $USER
sudo -u $real_user cp ../cuttlefish-*.deb ./out/

echo "[Install] Building cuttlefish VM image..."
sudo -u $real_user ./build.sh --verbose
cd "${WORKDIR}"; 

echo ""
echo "REBOOT REQUIRED: Matrisea installed successfully. Reboot to load additional kernel modules and apply udev rules."
echo ""
echo "After reboot, run docker-compose up and visit the url below to access the web panel:"
echo ""
echo "      http://127.0.0.1:10080/"
echo ""