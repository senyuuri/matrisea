![matrisea-logo](https://user-images.githubusercontent.com/2610986/125149686-f9499a00-e16c-11eb-8af9-531d4331ddae.png)

![license-mit](https://img.shields.io/badge/license-MIT-green)
![release](https://img.shields.io/badge/release-pre--alpha-lightgrey)

Matrisea (/ˈmeɪtrɪksiː/) is a cloud-based Android reversing platform that provides high-fidelity virtural devices with powerful integrated tools. 

![demo](./docs/demo.gif)

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

**System Requirements**

Matrisea is a web service that runs on both bare metal machines and VMs. However if you intend to use a VM through VirtualBox/VMware Workstation/ESXi, make sure to expose hardware-assisted virtualization to the guest OS.

Matrisea only supports Ubuntu at the moment.

Other pre-requisites:
- golang
- docker - *make sure docker can be managed by a non-root user [\[more details\]](https://docs.docker.com/engine/install/linux-postinstall/#manage-docker-as-a-non-root-user)*


**Installation**

> *For VMware users: Matrisea isn't compatible with vmware-tools due to vsock conflicts. The setup scripts will ask your permission to uninstall it. This won't affect the normal function of the VM.*

```
git clone https://github.com/senyuuri/matrisea
cd matrisea; ./setup.sh

# reboot to install kernel modules and apply udev rules
# after reboot, run docker-compose and visit http://127.0.0.1:10080/
docker-compose up -d
```

## Development

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

**Start the frontend server**
```
sudo apt install -y npm
sudo npm install -g yarn
cd web && yarn install

yarn start
```

**Start the backend server**

If you intend to use a remote dev machine, add the host IP in `api/api.go`
```
func main() {
    ...
	config.AllowOrigins = []string{"http://localhost:3000", "http://your_ip_here:3000"}
    ...
}
```

Specify a local `DATA_DIR` for runtime storage (configs, images, VMs etc.)
```
cd api && DATA_DIR={...} go run .
```
> *For VSCode Users*
> 
> *`gopls` in VSCode can't corretly identify imports for go modules in subfolders. To resolve "cannot find packages" warnings, goto `File > Add folder to workspace` and import folder `api` and `vmm` respectively.*
> *The [issue](https://github.com/golang/go/issues/32394) has been discussed in the community and is currently WIP.*



## Architecture
Matrisea is built on top of a variety of open source technologies.
- Frontend: React, novnc, xterm.js
- Backend: Golang, Gin
- VM: crosvm-backed cuttlefish AVD, KVM
- Orchestration: docker
- Android OS: AOSP GSI images

![architecture](./docs/architecture.png)

## Troubleshooting ##
**Upgrade vulnerable frontend packages**
Since yarn doesn't provide a `yarn audit fix`-like utility, we can use `npm` to generate a patched `package.json` then re-import to yarn. 

```
npm i --package-lock-only
npm audit fix
rm yarn.lock
yarn import --ignore-engines
rm package-lock.json
```