![matrisea-logo](https://user-images.githubusercontent.com/2610986/125149686-f9499a00-e16c-11eb-8af9-531d4331ddae.png)

![license-mit](https://img.shields.io/badge/license-MIT-green)
![release](https://img.shields.io/badge/release-pre--alpha-lightgrey)

## Matrisea

Matrisea (/ˈmeɪtrɪksiː/) is a cloud-based Android reversing platform that provides high-fidelity virtural devices with powerful integrated tools. 

**Features**
- Provide high fidelity virtual devices based on android-cuttlefish (crosvm+KVM) that guarantees full fidelity with Android framework
- Spin up multiple devices on demand and run them remotely or locally
- Support the latest AOSP (API level 28+) and Android mainline kernel up to 5.10
- Provision a device with ready-to-use reserving and researching tools e.g. adeb, bcc/eBPF, Frida
- Android customisation make easy
    - Provide a simple workflow to make your own base device e.g. upload custom kernels/AOSP images or install additional tools
    - Provide a tool to download pre-built artifacts from Android CI
- Provide a web UI to manage the device fleet and to access devices' VNC stream and interactive shell

## Quick Start
```
git clone https://github.com/senyuuri/matrisea
cd matrisea; ./setup.sh

# reboot to install kernel modules and apply udev rules
# after reboot, run docker-compose and visit http://127.0.0.1:10080/
docker-compose up -d
```

