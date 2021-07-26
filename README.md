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

## Development

**Prerequisites**
- go v1.16+
- docker v20+

**Preparation**
1. Clone the repo and build cuttlefish image. Once finished, reboot to load additional kernel modules and apply udev rules.
    ```
    git clone https://github.com/senyuuri/matrisea
    cd matrisea; ./setup.sh
    ```
2. To download a ready-made AOSP image for testing, Goto https://ci.android.com/ and search for branch `aosp-android11-gsi`. Among all the builds, look for a successful build (green box) under the `userdebug - aosp_cf_x86_x64_phone` column. Click on `Artifacts` and download the following files:
    - `aosp_cf_x86_64_phone-img-xxxxxxx.zip`
    - `cvd-host_package.tar.gz`

3. Create an `images` folder under the root of the source code. Copy both files from (2) into it and unzip to the current directory.
   ```
   cp aosp_cf_x86_64_phone-img-xxxxxxx.zip matrisea/images
   cp cvd-host_packages.tar.gz matrisea/images
   cd matrisea/images
   tar xvf cvd-host_package.tar.gz
   unzip aosp_cf_x86_64_phone-img-xxxxxx.zip
   ```

**Front End**
```
# install dependencies
sudo apt install -y npm
sudo npm install -g yarn
cd frontend && yarn install

# start the development server
yarn start
```

**Back End**
```
cd backend/api
go run .
```